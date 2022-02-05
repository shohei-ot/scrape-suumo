package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
	"github.com/slack-go/slack"
)

const (
	TMP_DIR_NAME  = ".tmp"
	TMP_FILE_NAME = "scrape-suumo-go.json"
)

type CliArgs struct {
	suumoUrl     string
	slackToken   string
	slackChannel string
	useSlack     bool
}

type TmpData struct {
	ApartmentsNumber     int    `json:"apartmentsNumber"`
	SearchTargetAreaHtml string `json:"searchTargetAreaHtml"`
}

var (
	slackApi                  *slack.Client
	dom                       *goquery.Selection
	args                      CliArgs
	currentMemo               TmpData
	existApartmentsNumber     bool
	apartmentsNumber          int
	existSearchTargetAreaHtml bool
	searchTargetAreaHtml      string
)

func main() {
	parseFlags()
	initSlack()

	initTmps()

	c := colly.NewCollector()

	registEvents(c)

	// url := "https://suumo.jp/jj/chintai/ichiran/FR301FC001/?ar=030&bs=040&fw2=&pc=30&po1=09&po2=99&ta=13&sc=13107&sc=13108&sc=13109&sc=13111&sc=13112&sc=13114&sc=13115&sc=13204&sc=13206&sc=13208&md=02&md=03&md=04&md=05&md=06&md=07&md=08&md=09&md=10&md=11&md=12&md=13&md=14&cb=7.0&ct=13.5&et=15&mb=40&mt=9999999&cn=25&co=1&kz=1&kz=2&kz=3&tc=0401303&tc=0401304&tc=0401301&tc=0401302&tc=0400101&tc=0400103&tc=0400104&tc=0400501&tc=0400502&tc=0400503&tc=0400601&tc=0400301&tc=0400302&tc=0400203&tc=0400205&tc=0400902&tc=0400906&tc=0400907&tc=0400912&tc=0400803&tc=0401106&tc=0400401&tc=0400702&shkr1=02&shkr2=02&shkr3=02&shkr4=02&shkk1=02060202&shkk1=02060203"
	url := args.suumoUrl

	if len(url) == 0 || strings.Index(url, "https://") != 0 {
		errMsg := "Require -ual flag. expect \"https://...\", actual: " + url
		postToSlack(errMsg)
		log.Fatal(errMsg)
	}

	c.Visit(url)
}

func parseFlags() {
	flag.StringVar(&args.suumoUrl, "url", "", "suumo url of the search result")
	flag.StringVar(&args.slackToken, "token", "", "slack access token")
	flag.StringVar(&args.slackChannel, "channel", "", "slack channel name")
	flag.Parse()

	args.useSlack = len(args.slackToken) > 0 && len(args.slackChannel) > 0
}

func registEvents(c *colly.Collector) {
	c.OnRequest(func(r *colly.Request) {
		fmt.Println("Visiting: ", r.URL)
	})

	c.OnHTML("body", func(h *colly.HTMLElement) {
		dom = h.DOM
	})

	c.OnHTML("#js-condTop-panel", func(h *colly.HTMLElement) {
		html, e := h.DOM.Html()
		if e != nil {
			postToSlack(e.Error())
			log.Fatal(e.Error())
		}
		// fmt.Println("HTML: ", html)
		searchTargetAreaHtml = html
		existSearchTargetAreaHtml = true
	})

	c.OnHTML(".paginate_set-hit", func(h *colly.HTMLElement) {
		str := h.Text
		m := regexp.MustCompile(`\s*(\d{1,})件`)
		matches := m.FindAllStringSubmatch(str, -1)
		if len(matches) == 0 {
			errMsg := "no matches: " + str
			postToSlack(errMsg)
			log.Fatal(errMsg)
		}
		numStr := matches[0][1]
		num, e := strconv.Atoi(numStr)
		if e != nil {
			postToSlack(e.Error())
			log.Fatal(e.Error())
		}
		apartmentsNumber = num
		existApartmentsNumber = true
	})

	c.OnScraped(func(r *colly.Response) {
		// fmt.Println("Scraped.")
		if !existApartmentsNumber || !existSearchTargetAreaHtml {
			errMsg := "failed scraping the target html: " + string(r.Body)
			postToSlack(errMsg)
			log.Fatal(errMsg)
		}
		afterScrapedProcess()
	})
}

func tmpDirPath() string {
	ex, err := os.Executable()
	if err != nil {
		postToSlack(err.Error())
		log.Fatal(err.Error())
	}
	// baseDir, e := os.Getwd()
	// baseDir := os.TempDir()
	// if e != nil {
	// 	postToSlack(e.Error())
	// 	log.Fatal(e.Error())
	// }
	baseDir := filepath.Dir(ex)
	return filepath.Join(baseDir, TMP_DIR_NAME)
}

func tmpFilePath() string {
	return filepath.Join(tmpDirPath(), TMP_FILE_NAME)
}

func initTmps() {
	initTmpDir()
	initTmpFile()
}

func initTmpDir() {
	tmpDir := tmpDirPath()
	if f, e := os.Stat(tmpDir); os.IsNotExist(e) || !f.IsDir() {
		if e := os.Mkdir(tmpDir, 0766); e != nil {
			postToSlack(e.Error())
			log.Fatal(e.Error())
		}
	}
}

func initTmpFile() {
	tmpFilePath := tmpFilePath()
	if f, e := os.Stat(tmpFilePath); os.IsNotExist(e) || f.IsDir() {
		data := new(TmpData)
		saveTmpData(data)
	}
}

func memoCurrentData(apartmentsNumber int, searchTargetAreaHtml string) TmpData {
	currentMemo.ApartmentsNumber = apartmentsNumber
	currentMemo.SearchTargetAreaHtml = searchTargetAreaHtml

	return currentMemo
}

func loadTmpData() TmpData {
	bytes, e := os.ReadFile(tmpFilePath())
	if e != nil {
		postToSlack(e.Error())
		log.Fatal(e.Error())
	}
	var prevData TmpData
	if e := json.Unmarshal(bytes, &prevData); e != nil {
		postToSlack(e.Error())
		log.Fatal(e.Error())
	}
	return prevData
}

func saveTmpData(d *TmpData) {
	bytes, e := json.Marshal(d)
	if e != nil {
		postToSlack(e.Error())
		log.Fatal(e.Error())
	}

	if e := os.WriteFile(tmpFilePath(), bytes, 0766); e != nil {
		postToSlack(e.Error())
		log.Fatal(e.Error())
	}
}

func existDiff(current TmpData, prev TmpData) bool {
	return current.ApartmentsNumber != prev.ApartmentsNumber ||
		current.SearchTargetAreaHtml != prev.SearchTargetAreaHtml
}

func afterScrapedProcess() {
	memoCurrentData(apartmentsNumber, searchTargetAreaHtml)
	if !existDiff(currentMemo, loadTmpData()) {
		msg := "new apartments NOT found"
		postToSlack(msg)
		fmt.Println(msg)
		os.Exit(0)
	}
	saveTmpData(&currentMemo)
	msg := "new apartments found!"
	postToSlack(msg)
	fmt.Println(msg)
}

func initSlack() {
	if args.useSlack {
		slackApi = slack.New(args.slackToken)
	}
}

func postToSlack(message string) {
	if slackApi != nil {
		ch, ts, err := slackApi.PostMessage(args.slackChannel, slack.MsgOptionText(message, true))
		if err != nil {
			log.Fatal(err.Error())
		}
		fmt.Println("channel="+ch, "timestamp="+ts)
	}
}
