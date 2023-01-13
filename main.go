package main

import (
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
)

const MdTemplate = `---
title: "{{ .Title }}"
subtitle: ""
date: {{ .Date }}
lastmod: {{ .LastMod }}
draft: {{ .Draft }}
author: "SoulChild"
authorLink: "https://www.soulchild.cn"
description: ""
license: "本站使用「署名 4.0 国际」创作共享协议，可自由转载、引用，但需署名作者且注明文章出处"
tags: [{{if .Tags}}"{{ join .Tags "\",\""}}"{{end}}]
categories: [{{if .Categories}}"{{ join .Categories "\",\""}}"{{end}}]
comment:
  enable: true
password: ""
message: "请输入密码"
slug: {{ .Slug }}
---
<!--more-->
{{ .Content }}
`

var CONN *SqlObj

type TypechoContent struct {
	Cid          uint    `db:"cid"`
	Title        string  `db:"title"`
	Slug         string  `db:"slug"`
	Created      uint    `db:"created"`
	Modified     uint    `db:"modified"`
	Text         string  `db:"text"`
	Order        uint    `db:"order"`
	Authorid     uint    `db:"authorId"`
	Template     *string `db:"template"`
	Type         string  `db:"type"`
	Status       string  `db:"status"`
	Password     *string `db:"password"`
	Commentsnum  uint    `db:"commentsNum"`
	Allowcomment string  `db:"allowComment"`
	Allowping    string  `db:"allowPing"`
	Allowfeed    string  `db:"allowFeed"`
	Parent       uint    `db:"parent"`
	Views        int     `db:"views"`
}

type TypechoMeta struct {
	Mid         uint    `db:"mid"`
	Name        string  `db:"name"`
	Slug        string  `db:"slug"`
	Type        string  `db:"type"`
	Description *string `db:"description"`
	Count       uint    `db:"count"`
	Order       uint    `db:"order"`
	Parent      uint    `db:"parent"`
}

type Article struct {
	Content    TypechoContent
	Tags       []string
	Categories []string
}

type SqlObj struct {
	MysqlHost            string
	MysqlPort            uint16
	MysqlUser, MysqlPass string
	Database             string
	DB                   *sqlx.DB
}

type Page struct {
	Title      string
	Date       time.Time
	LastMod    time.Time
	Draft      bool
	Tags       []string
	Categories []string
	Content    string
	Slug       string
}

func (conn *SqlObj) InitDB() (err error) {
	// DSN(Data Source Name) 数据库连接字符串
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True", conn.MysqlUser, conn.MysqlPass, conn.MysqlHost, conn.MysqlPort, conn.Database)
	// 注册第三方mysql驱动到sqlx中并连接到dsn数据源设定的数据库中(与database/sql不同点，代码更加精简)
	conn.DB, err = sqlx.Connect("mysql", dsn)
	if err != nil {
		fmt.Printf("Connect %s DB Failed\n%v \n", dsn, err)
		return err
	}
	// 设置与数据库建立连接的最大数目
	conn.DB.SetMaxOpenConns(1024)
	// 设置连接池中的最大闲置连接数
	conn.DB.SetMaxIdleConns(10)
	return nil
}

func (conn *SqlObj) queryAllCateGory() (categories []TypechoMeta) {
	sqlStr := `select name from typecho_metas where type = ?`

	err := conn.DB.Select(&categories, sqlStr, "category")
	if err != nil {
		fmt.Printf("query failed, err:%v\n", err)
		return
	}
	return
}

func (conn *SqlObj) queryAllArticle() (articles []Article) {
	sqlStr := `select * from typecho_contents where type in (?,?) order by created desc`
	var contents []TypechoContent
	err := conn.DB.Select(&contents, sqlStr, "post", "post_draft")
	if err != nil {
		fmt.Printf("query failed, err:%v\n", err)
		return
	}

	for _, item := range contents {
		article := Article{Content: item}
		err := conn.queryMeta(&article)
		if err != nil {
			fmt.Println("查询文章tag出错", err.Error())
			return nil
		}
		articles = append(articles, article)
	}
	return
}

func (conn *SqlObj) queryMeta(article *Article) error {
	sqlStr := `select m.mid,m.name,m.type
				from typecho_relationships r 
				join typecho_metas As m On r.mid = m.mid
				where r.cid = ?`

	var tags []TypechoMeta
	err := conn.DB.Select(&tags, sqlStr, article.Content.Cid)
	if err != nil {
		fmt.Printf("query failed, err:%v\n", err)
		return err
	}

	for _, item := range tags {
		if item.Type == "tag" {
			article.Tags = append(article.Tags, item.Name)
		} else if item.Type == "category" {
			article.Categories = append(article.Categories, item.Name)
		}
	}

	return nil
}

func writeFile(path string, content string) {
	// 1。打开文件
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 644)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	return

}

func copyFile(srcPath, dstPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// 创建文件目录
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		fmt.Println("目录创建失败", err.Error())
	}

	// 创建目标文件
	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// 使用 io.Copy 函数复制文件
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	return nil
}

func mdHandler(article Article) string {
	// 图片处理
	re := regexp.MustCompile(`(usr|wp-content)/uploads/\d{4}/\d{2}/(.*?\.(png|jpg|jpeg))`)
	for _, match := range re.FindAllStringSubmatch(article.Content.Text, -1) {
		srcPath := match[0]
		dstPath := filepath.Join("./content/posts/", article.Categories[0], article.Content.Title, "images/", match[2])
		//fmt.Println(srcPath, "\t", dstPath)
		err := copyFile(srcPath, dstPath)
		if err != nil {
			fmt.Println("复制图片出错", err.Error())
			return ""
		}
	}

	// 图片路径修改及删除多余文本
	re = regexp.MustCompile(`http.*?(usr|wp-content)/uploads/\d{4}/\d{2}/(.*?\.(png|jpg|jpeg))`)
	for _, match := range re.FindAllStringSubmatch(article.Content.Text, -1) {
		originPath := match[0]
		newPath := "images/" + match[2]
		article.Content.Text = strings.Replace(article.Content.Text, originPath, newPath, -1)
	}
	//article.Content.Text = strings.Replace(article.Content.Text, "https://soulchild.cn/usr/uploads/", "usr/uploads/", -1)
	//article.Content.Text = strings.Replace(article.Content.Text, "https://www.soulchild.cn/usr/uploads/", "usr/uploads/", -1)
	article.Content.Text = strings.Replace(article.Content.Text, "<!--markdown-->", "", 1)

	return article.Content.Text
}

func clearSymbol(s *string) {
	symbol := []string{
		":",
		`\`,
		`/`,
		`*`,
		`?`,
		`"`,
		`<`,
		`>`,
		`|`,
	}
	for _, item := range symbol {
		*s = strings.ReplaceAll(*s, item, "")
	}
	*s = strings.TrimSpace(*s)
}

func generateMd(articles []Article) {
	funcs := template.FuncMap{"join": strings.Join}
	tmpl, err := template.New("mdTemplate").Funcs(funcs).Parse(MdTemplate)
	if err != nil {
		fmt.Println(err)
		return
	}

	for i, item := range articles {
		draft := false
		if item.Content.Status != "publish" || item.Content.Type == "post_draft" {
			draft = true
		}

		content := mdHandler(item)

		page := Page{
			Title:      item.Content.Title,
			Date:       time.Unix(int64(item.Content.Created), 0),
			LastMod:    time.Unix(int64(item.Content.Modified), 0),
			Draft:      draft,
			Tags:       item.Tags,
			Categories: item.Categories,
			Content:    content,
			Slug:       strconv.Itoa(int(item.Content.Cid)),
		}

		title := item.Content.Title
		clearSymbol(&title)
		mdPath := filepath.Join("./content/posts/", item.Categories[0], title, "index.md")
		if err := os.MkdirAll(filepath.Dir(mdPath), 0755); err != nil {
			fmt.Println("md目录创建失败", err.Error())
		}

		file, err := os.OpenFile(mdPath, os.O_CREATE|os.O_WRONLY, 644)
		if err != nil {
			fmt.Println("打开md失败", err.Error())
			fmt.Println(err.Error())
			return
		}
		defer file.Close()

		err = tmpl.Execute(file, page)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		fmt.Println(item.Content.Title, "创建成功", i)
	}

}

func init() {
	CONN = &SqlObj{
		MysqlHost: "192.168.124.52",
		MysqlPort: 3306,
		MysqlUser: "root",
		MysqlPass: "xxx",
		Database:  "typecho",
	}

	if err := CONN.InitDB(); err != nil {
		fmt.Println("数据库初始化失败", err.Error())
		return
	}

	if err := os.MkdirAll("./content/posts", 0755); err != nil {
		fmt.Println("目录创建失败", err.Error())
	}
}

func main() {
	defer CONN.DB.Close()

	var articles []Article
	articles = CONN.queryAllArticle()

	//for _, v := range articles {
	//	fmt.Printf("ID:%d\tTitle:%s\tTags:%v\tCates:%v\t\n", v.Content.Cid, v.Content.Title, v.Tags, v.Categories)
	//}
	generateMd(articles)
}
