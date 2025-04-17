package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yudai/gojsondiff"
	"github.com/yudai/gojsondiff/formatter"
)

type DiffRequest struct {
	URL1 string `json:"url1" binding:"required,url"`
	URL2 string `json:"url2" binding:"required,url"`
}

type result struct {
	Body []byte
	Err  error
}

func main() {
	r := gin.Default()

	// 提供 static 目录下的文件：index.html 和 latest.json
	r.Static("/static", "./static")
	r.GET("/index", func(c *gin.Context) {
		c.File("static/index.html")
	})

	// 映射 /hand 到 static/hand.html
	r.GET("/hand", func(c *gin.Context) {
		c.File("static/hand.html")
	})
	// 接收对比请求，返回 JSON 并写入 latest.json
	r.POST("/api/diff", diffHandler)

	r.Run(":8080")
}

func diffHandler(c *gin.Context) {
	var req DiffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 并发获取两个服务 JSON
	ch1 := make(chan result)
	ch2 := make(chan result)
	client := &http.Client{Timeout: 10 * time.Second}
	go func() { ch1 <- fetchJSON(client, req.URL1) }()
	go func() { ch2 <- fetchJSON(client, req.URL2) }()

	res1 := <-ch1
	res2 := <-ch2
	if res1.Err != nil || res2.Err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error1": errorString(res1.Err), "error2": errorString(res2.Err)})
		return
	}

	// 计算 diff
	d := gojsondiff.New()
	diffObj, err := d.Compare(res1.Body, res2.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 格式化 diff
	var doc map[string]interface{}
	json.Unmarshal(res1.Body, &doc)
	formatter := formatter.NewAsciiFormatter(doc, formatter.AsciiFormatterConfig{ShowArrayIndex: true, Coloring: false})
	diffText, _ := formatter.Format(diffObj)

	// 构造返回结构
	payload := gin.H{
		"raw1": jsonRaw(res1.Body),
		"raw2": jsonRaw(res2.Body),
		"diff": diffText,
	}

	// 将 payload 写入 static/latest.json
	if b, err := json.MarshalIndent(payload, "", "  "); err == nil {
		path := filepath.Join("static", "latest.json")
		ioutil.WriteFile(path, b, os.ModePerm)
	}

	// 返回给调用者
	c.JSON(http.StatusOK, payload)
}

func fetchJSON(client *http.Client, url string) result {
	resp, err := client.Get(url)
	if err != nil {
		return result{Err: err}
	}
	defer resp.Body.Close()
	b, _ := ioutil.ReadAll(resp.Body)
	return result{Body: b}
}

func jsonRaw(b []byte) interface{} {
	var v interface{}
	json.Unmarshal(b, &v)
	return v
}

func errorString(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
