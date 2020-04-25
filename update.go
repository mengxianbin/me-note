package main

import (
	"container/list"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
)

var outDir = "site"
var ignoredDirReg = regexp.MustCompile(fmt.Sprintf("(\\.|%s)", outDir))
var ignoredFileReg = regexp.MustCompile("(^\\d)|(^\\.git)|(^_)|(^index)|README|LICENSE|update.go")

func uriEncode(input string) string {
	return url.PathEscape(input)
}

func getMarkdownContent(home string, parent string, fileName string, title string, ext string) string {
	// 处理图片
	if strings.Contains(parent, "Pictures") {
		return fmt.Sprintf("![%s](%s/%s/%s)", title, home, parent, uriEncode(fileName))
	}

	// 读取原始文件
	fi, err := os.Open(fmt.Sprintf("%s/%s", parent, fileName))
	if err != nil {
		log.Printf("Markdown reading error: %#v\n", err)
		panic(err)
	}
	defer func() { _ = fi.Close() }()
	fd, err := ioutil.ReadAll(fi)
	content := string(fd)

	// 添加 markdown 块标记
	if ext != ".md" {
		return fmt.Sprintf("```%s\n%s\n```\n", ext[1:], content)
	}

	return content
}

func writeMarkdown(parent string, fileName string, home string, parents *list.List) (title string, err error) {
	// 创建多级目录
	newParent := fmt.Sprintf("%s/%s", outDir, parent)
	err = os.MkdirAll(newParent, 0777)
	if err != nil {
		log.Printf("Directory making error: %#v\n", err)
		panic(err)
	}

	// 创建新文件名
	ext := path.Ext(fileName)
	title = fileName[:len(fileName)-len(ext)]
	newFileName := title + ".md"

	// 创建 markdown
	newPath := newParent + "/" + newFileName
	file, err := os.Create(newPath)
	if err != nil {
		log.Printf("Markdown creating error: %#v\n", err)
		panic(err)
	}

	// 添加上级目录链接
	parents.PushBack(title)
	pathLinks, _ := toPathLink(home, parents)
	parents.Remove(parents.Back())
	_, err = file.WriteString(pathLinks)

	// 添加正文偏移
	_, err = file.WriteString("\n\n")

	// 添加原始文件内容
	root := fmt.Sprintf("%s/%s", home, parents.Front().Value.(string))
	content := getMarkdownContent(root, parent, fileName, title, ext)
	if len(strings.Trim(content, "")) == 0 {
		content = "# TO DO\n"
	}
	_, err = file.WriteString(content)

	// 关闭文件
	err = file.Close()

	return
}

func toPathLink(home string, parents *list.List) (full string, last string) {
	// home
	link := fmt.Sprintf("[Home](%s)", home)

	// repo / content
	repo := parents.Front()
	repoName := repo.Value.(string)
	path := fmt.Sprintf("%s/%s/%s", home, repoName, outDir)
	link += fmt.Sprintf(" /\n[%s](%s)", repoName, path)

	// path list
	for p := repo.Next(); p != nil; p = p.Next() {
		path = fmt.Sprintf("%s/%s", path, uriEncode(p.Value.(string)))
		link += fmt.Sprintf(" /\n[%s](%s)", p.Value.(string), path)
	}

	return link, path
}

// GenerateIndex 为目录递归生成 index 文件
func GenerateIndex(path string, home string, parents *list.List) (err error) {
	// 生成索引目录
	indexDir := fmt.Sprintf("%s/%s", outDir, path)
	err = os.MkdirAll(indexDir, 0777)
	if err != nil {
		log.Printf("Directory making error: %#v\n", err)
		panic(err)
	}

	// 创建 index 文件
	indexPath := fmt.Sprintf("%s/index.md", indexDir)
	indexFile, err := os.Create(indexPath)
	if err != nil {
		log.Printf("Index creating error: %s, %#v\n", indexPath, err)
		panic(err)
	}

	// 添加上级目录链接
	pathLinks, parentLink := toPathLink(home, parents)
	_, err = indexFile.WriteString(pathLinks)

	// 定义结束操作
	itemCount := 0
	defer func() {
		if itemCount == 0 {
			// 生成 TO DO
			_, err = indexFile.WriteString("\n\n# TO DO")

			// 生成 .gitkeep 文件
			keep, _ := os.Create(fmt.Sprintf("%s/.gitkeep", path))
			err = keep.Close()
		} else {
			// 移除冗余的 .gitkeep 文件
			os.Remove(fmt.Sprintf("%s/.gitkeep", path))
		}

		// 添加结束行
		_, err = indexFile.WriteString("\n")

		// 关闭索引文件
		err = indexFile.Close()
		log.Printf("Index file closed: path=%s, itemCount=%d.", path, itemCount)
	}()

	// 读取文件列表
	files, err := ioutil.ReadDir(path)
	if err != nil {
		log.Printf("Directory reading error: %#v\n", err)
		panic(err)
	}

	// 遍历文件名，追加到 index 文件
	for _, file := range files {
		if strings.HasPrefix(file.Name(), ".") {
			continue
		}

		// 拼接链接相对路径
		var linkTitle string
		var linkPath string
		if file.IsDir() { // 处理目录
			if ignoredDirReg.MatchString(file.Name()) {
				continue
			}

			parents.PushBack(file.Name())
			err = GenerateIndex(path+"/"+file.Name(), home, parents)
			parents.Remove(parents.Back())

			linkTitle = file.Name()
			linkPath = fmt.Sprintf("%s/%s/", parentLink, uriEncode(linkTitle))
		} else { // 处理文件
			if ignoredFileReg.MatchString(file.Name()) {
				continue
			}

			linkTitle, err = writeMarkdown(path, file.Name(), home, parents)
			linkPath = fmt.Sprintf("%s/%s", parentLink, uriEncode(linkTitle))
		}

		// 生成链接
		link := fmt.Sprintf("## [%s](%s)", linkTitle, linkPath)
		log.Printf("Link: %s\n", link)

		// 写入链接
		_, err := indexFile.WriteString("\n\n" + link)
		if err != nil {
			err := indexFile.Close()
			log.Printf("Link writing error: %#v\n", err)
			panic(err)
		}

		// 累计有效文件数
		itemCount++
	}

	return err
}

func main() {
	// 清理旧索引文件
	_ = os.RemoveAll(outDir)

	// 获取仓库名称
	wd, err := os.Getwd()
	if err != nil {
		log.Printf("Repo name error: %#v\n", err)
		panic(err)
	}
	repoName := path.Base(wd)

	// 生成新索引文件
	parents := list.New()
	parents.PushBack(repoName)
	_ = GenerateIndex(".", "https://mengxianbin.github.io", parents)
}
