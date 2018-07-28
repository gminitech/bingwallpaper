package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/antchfx/antch"
	"github.com/antchfx/jsonquery"
)

var (
	regions = []string{
		"de-de",
		"en-au",
		"en-ca",
		"en-gb",
		"en-in",
		"en-us",
		"fr-fr",
		"ja-jp",
		"zh-cn",
	}
)

type wallpaper struct {
	Region string
	URL    string
}

type wallpaperSpider struct{}

func (s *wallpaperSpider) ServeSpider(c chan<- antch.Item, res *http.Response) {
	doc, err := jsonquery.Parse(res.Body)
	if err != nil {
		return
	}
	mkt := res.Request.URL.Query().Get("mkt")
	for _, n := range jsonquery.Find(doc, "//images/*") {
		v := &wallpaper{Region: mkt}
		path := n.SelectElement("url").InnerText()
		u, _ := res.Request.URL.Parse(path)
		v.URL = u.String()

		if strings.LastIndex(v.URL, "1366") > 0 {
			fmt.Println(res.Request.URL)
		}
		c <- v
	}
}

type imagesPipeline struct {
	rootPath string
	next     antch.PipelineHandler
	m        sync.Mutex
}

func (pipe *imagesPipeline) ServePipeline(v antch.Item) {
	item := v.(*wallpaper)
	i := strings.LastIndex(item.URL, "/")
	file := item.URL[i+1:]
	a := strings.Split(file, "_")
	a = append(a[:1], a[2:]...)
	file = strings.Join(a, "_")
	path := filepath.Join(pipe.rootPath, file)

	pipe.m.Lock()
	defer pipe.m.Unlock()

	if _, err := os.Stat(path); err == nil {
		return
	}

	resp, err := http.Get(item.URL)
	if err != nil {
		// image download failed.
		logrus.Warnf("wallpaper file %s download failed", item.URL)
		return
	}
	defer resp.Body.Close()

	out, err := os.Create(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer out.Close()
	io.Copy(out, resp.Body)
	logrus.Infof("wallpaper file %s download successful", item.URL)
}

func run(imagePath string, quit chan struct{}) {
	crawler := &antch.Crawler{Exit: quit}
	crawler.UseCompression()

	crawler.Handle("*", &wallpaperSpider{})
	crawler.UsePipeline(
		antch.Pipeline(func(next antch.PipelineHandler) antch.PipelineHandler {
			return &imagesPipeline{rootPath: imagePath}
		}),
	)

	var startURLs []string
	for _, region := range regions {
		homepageURL := fmt.Sprintf("https://www.bing.com/HPImageArchive.aspx?format=js&idx=0&n=%d&pid=hp&mkt=%s", 10, region)
		startURLs = append(startURLs, homepageURL)
	}
	go crawler.StartURLs(startURLs)
	timer := time.NewTicker(5 * time.Second)
loop:
	for {
		select {
		case <-timer.C:
			go crawler.StartURLs(startURLs)
		case <-quit:
			break loop
		}
	}
	timer.Stop()
}

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	quit := make(chan struct{})
	// Create new folder if its not exists.
	os.Mkdir("./wallpapers/", 0777)
	// running
	go run("./wallpapers/", quit)
	<-sigs
	close(quit)
	fmt.Println("exiting")
}
