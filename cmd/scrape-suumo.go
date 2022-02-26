package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
	"github.com/slack-go/slack"
)

const (
	TMP_DIR_NAME      = ".cache"
	OLD_TMP_FILE_NAME = "scrape-suumo-v20220207T0145.json"
	IGNORE_FILE_NAME  = "scrape-suumo-ignore-v20220208T2122.json"
	VERSION           = "v0.20220226.1759"
)

type CliArgs struct {
	suumoUrl     string
	slackToken   string
	slackChannel string
	useSlack     bool
	noCache      bool
	noSlack      bool
	version      bool
}

type Suumo struct {
	Hit        int    `json"hit"`
	CondArea   string `json"cond_area"`
	TotalPage  int
	Apartments []*Apartment `json:"apartments"`
}

type IgnoredApartment struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type Apartment struct {
	Name          string   `json:"name"`
	Address       string   `json:"address"`
	Transports    []string `json:"transports"`
	AgeOfBuilding string   `json:"age_of_building"`
	TotalFloor    string   `json:"total_floor"`
	Rooms         []*Room  `json:"rooms"`
}

type Room struct {
	Thumbnail     string `json:"thumbnail"`
	Floor         string `json:"floor"`
	RentPrice     string `json:"rent_price"`
	AdminPrice    string `json:"admin_price"`
	DepositPrice  string `json:"deposit_price"`
	GratuityPrice string `json:"gratuity_price"`
	Madori        string `json:"madori"`
	Menseki       string `json:"menseki"`
	Url           string `json:"url"`
}

var (
	slackApi *slack.Client
	args     CliArgs
)

func initIgnoredFile(refresh bool) {
	ignoreFilePath := getIgnoreFilePath()
	var ignoreFileCreated bool
	// 無かったら作る
	if s, e := os.Stat(ignoreFilePath); os.IsNotExist(e) || s.IsDir() || refresh {
		ignoredApartments := []*IgnoredApartment{}
		jsonBytes, e := json.Marshal(ignoredApartments)
		if e != nil {
			log.Fatal(e.Error())
		}
		e = os.WriteFile(ignoreFilePath, jsonBytes, 0700)
		if e != nil {
			log.Fatal(e.Error())
		}
		ignoreFileCreated = true
	}

	// old cache があれば移行して削除
	if ignoreFileCreated {
		if s, e := os.Stat(tmpFilePath()); !os.IsNotExist(e) && !s.IsDir() {
			_migrateToIgnoreFile(loadPrevSuumo())
			_rmOldCacheFile()
		}
	}
}

func _migrateToIgnoreFile(sm *Suumo) {
	ignoreFilePath := getIgnoreFilePath()
	ignoreDataBytes, e := os.ReadFile(ignoreFilePath)
	if e != nil {
		log.Fatal(e.Error())
	}
	ignoreData := []*IgnoredApartment{}
	if er := json.Unmarshal(ignoreDataBytes, &ignoreData); er != nil {
		log.Fatal(er.Error())
	}

	for _, ap := range sm.Apartments {
		ignAp := new(IgnoredApartment)
		ignAp.Name = ap.Name
		ignAp.Address = ap.Address
		ignoreData = append(ignoreData, ignAp)
	}

	writeIgnoredApartments(ignoreData)
}

func _rmOldCacheFile() {
	oldCacheFilePath := tmpFilePath()
	if s, e := os.Stat(oldCacheFilePath); os.IsNotExist(e) || s.IsDir() {
		return
	}

	e := os.Remove(oldCacheFilePath)
	if e != nil {
		log.Fatal(e.Error())
	}
}

func getIgnoreFilePath() string {
	tmpDir := tmpDirPath()
	return path.Join(tmpDir, IGNORE_FILE_NAME)
}

func getIgnoredApartments() []*IgnoredApartment {
	ignoreFilePath := getIgnoreFilePath()

	byts, e := os.ReadFile(ignoreFilePath)
	if e != nil {
		log.Fatal(e.Error())
	}
	entity := []*IgnoredApartment{}
	er := json.Unmarshal(byts, &entity)
	if er != nil {
		log.Fatal(e.Error())
	}
	return entity
}

func writeIgnoredApartments(apts []*IgnoredApartment) {
	ignoreFilePath := getIgnoreFilePath()

	byts, e := json.Marshal(apts)
	if e != nil {
		log.Fatal(e.Error())
	}
	e = os.WriteFile(ignoreFilePath, byts, 0700)
	if e != nil {
		log.Fatal(e.Error())
	}
}

func excludeOldApartments(sm *Suumo, ig []*IgnoredApartment) []*Apartment {
	var newestApartments []*Apartment
	for _, ap := range sm.Apartments {
		existsInIgnored := false
		if ig != nil {
			for _, igAp := range ig {
				eqApName := ap.Name == igAp.Name
				eqApAddr := ap.Address == igAp.Address
				if eqApName && eqApAddr {
					existsInIgnored = true
					break
				}
			}
		}
		if !existsInIgnored {
			newestApartments = append(newestApartments, ap)
		}
	}
	return newestApartments
}

func main() {
	parseFlags()
	initSlack()
	initTmpDir()
	initIgnoredFile(args.noCache)

	url := args.suumoUrl

	if len(url) == 0 || strings.Index(url, "https://") != 0 {
		errMsg := "Require -ual flag. expect \"https://...\", actual: " + url
		postToSlack(errMsg)
		log.Fatal(errMsg)
	}

	htmlCh := make(chan string)
	defer close(htmlCh)

	go fetcHtmlBody(htmlCh, url)

	htmlStr := <-htmlCh
	doc := parseToGoqueryDoc(htmlStr)

	sm := extractSuumo(doc)

	fmt.Printf("Apartments: %d\n", len(sm.Apartments))

	var totalRoom int
	for i := 0; i < len(sm.Apartments); i++ {
		rms := len(sm.Apartments[i].Rooms)
		totalRoom += rms
	}
	fmt.Printf("Total Rooms: %d\n", totalRoom)

	igAps := getIgnoredApartments()
	var diffData []*Apartment
	if args.noCache {
		diffData = sm.Apartments
	} else {
		diffData = excludeOldApartments(sm, igAps)
	}
	existDiff := len(diffData) > 0

	if !existDiff {
		msg := "新着物件なし"
		// postToSlack(msg)
		fmt.Println(msg)
		os.Exit(0)
	}

	fmt.Println("------------------------------------------------------------")

	saveTmpData(sm)

	msgs := genApartmentMessages(diffData)
	s := strings.Join(msgs, "\n------------------------------------------------------------\n")
	postToSlack(s)
	fmt.Println(s)

	for _, ap := range diffData {
		igp := new(IgnoredApartment)
		igp.Name = ap.Name
		igp.Address = ap.Address
		igAps = append(igAps, igp)
	}
	writeIgnoredApartments(igAps)
}

func parseFlags() {
	flag.StringVar(&args.suumoUrl, "url", "", "<required> suumo url of the search result")
	flag.StringVar(&args.slackToken, "token", "", "slack access token")
	flag.StringVar(&args.slackChannel, "channel", "", "slack channel name")

	flag.BoolVar(&args.version, "v", false, "display version")

	var noCache bool
	flag.BoolVar(&noCache, "refresh", false, "refresh suumo cache")

	var noSlack bool
	flag.BoolVar(&noSlack, "no-slack", false, "not use slack")

	flag.Parse()

	args.useSlack = len(args.slackToken) > 0 && len(args.slackChannel) > 0
	args.noCache = noCache
	args.noSlack = noSlack

	if args.version {
		fmt.Println(VERSION)
		os.Exit(0)
	}
}

func initSlack() {
	if args.useSlack {
		slackApi = slack.New(args.slackToken)
	}
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

func saveTmpData(d *Suumo) {
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

func postToSlack(message string) {
	if slackApi != nil && !args.noSlack {
		ch, ts, err := slackApi.PostMessage(args.slackChannel, slack.MsgOptionText(message, true))
		if err != nil {
			log.Fatal(err.Error())
		}
		fmt.Println("channel="+ch, "timestamp="+ts)
	}
}

func fetcHtmlBody(ch chan<- string, url string) {
	c := colly.NewCollector()
	c.OnScraped(func(r *colly.Response) {
		bodyStr := string(r.Body)
		ch <- bodyStr
	})
	c.Visit(url)
}

func parseToGoqueryDoc(html string) *goquery.Document {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		postToSlack(err.Error())
		log.Fatal(err.Error())
	}
	return doc
}

func extractSuumo(doc *goquery.Document) *Suumo {
	condArea := doc.Find("#js-condTop-panel").Text()
	hit := getApartmentHit(doc.Find(".paginate_set-hit").Text())
	apartments := extractApartments(doc.Selection)
	totalPage := getMaxPageNum(doc.Selection)

	return &Suumo{
		CondArea:   trim(condArea),
		Hit:        hit,
		TotalPage:  totalPage,
		Apartments: apartments,
	}
}

func getApartmentHit(str string) int {
	m := regexp.MustCompile(`\s*(\d{1,})件`)
	matches := m.FindAllStringSubmatch(str, -1)
	if len(matches) == 0 {
		msg := "no matches: " + str
		fmt.Println(msg)
		os.Exit(0)
		// postToSlack(errMsg)
		// log.Fatal(errMsg)
	}
	numStr := matches[0][1]
	num, e := strconv.Atoi(numStr)
	if e != nil {
		postToSlack(e.Error())
		log.Fatal(e.Error())
	}
	return num
}

func extractApartments(dom *goquery.Selection) []*Apartment {
	var apartments []*Apartment
	dom.Find("#js-bukkenList > ul.l-cassetteitem > li").Each(func(i int, s *goquery.Selection) {
		apartments = append(apartments, extractApartment(s))
	})
	return apartments
}

func extractApartment(divCassetteItem *goquery.Selection) *Apartment {
	name := divCassetteItem.Find(".cassetteitem_content-title").Text()
	addr := divCassetteItem.Find(".cassetteitem_detail-col1").Text()
	var transportList []string
	divCassetteItem.Find(".cassetteitem_detail-col2 > .cassetteitem_detail-text").Each(func(i int, el *goquery.Selection) {
		transportList = append(transportList, trim(el.Text()))
	})
	age := divCassetteItem.Find(".cassetteitem_detail-col3 > div:nth-child(1)").Text()
	totalFloorStr := divCassetteItem.Find(".cassetteitem_detail-col3 > div:nth-child(2)").Text()

	return &Apartment{
		Name:          trim(name),
		Address:       trim(addr),
		Transports:    transportList,
		AgeOfBuilding: trim(age),
		TotalFloor:    trim(totalFloorStr),
		Rooms:         extractRooms(divCassetteItem),
	}
}

func extractRooms(divCassetteItem *goquery.Selection) []*Room {
	var rooms []*Room
	tbodies := divCassetteItem.Find("div.cassetteitem-item > table.cassetteitem_other > tbody")
	tbodies.Each(func(i int, tbody *goquery.Selection) {
		rm := new(Room)

		imgEl := tbody.Find("div.casssetteitem_other-thumbnail > img")
		imgUrl, existImgSrc := imgEl.Attr("src")
		if existImgSrc {
			rm.Thumbnail = trim(imgUrl)
		}

		floorNumStr := tbody.Find("td:nth-child(3)").Text()
		floorNumStr = strings.Trim(floorNumStr, " ")
		rm.Floor = trim(floorNumStr)

		rentStr := tbody.Find(".cassetteitem_price--rent").Text()
		rm.RentPrice = trim(rentStr)

		adminStr := tbody.Find(".cassetteitem_price--administration").Text()
		rm.AdminPrice = trim(adminStr)

		depositStr := tbody.Find(".cassetteitem_price--deposit").Text()
		rm.DepositPrice = trim(depositStr)

		gratuityStr := tbody.Find(".cassetteitem_price--gratuity").Text()
		rm.GratuityPrice = trim(gratuityStr)

		madori := tbody.Find(".cassetteitem_madori").Text()
		rm.Madori = trim(madori)

		menseki := tbody.Find(".cassetteitem_menseki").Text()
		rm.Menseki = trim(menseki)

		urlPath, urlExist := tbody.Find("td").Last().Find("a").Attr("href")
		if urlExist {
			rm.Url = "https://suumo.jp" + trim(urlPath)
		}

		rooms = append(rooms, rm)
	})
	return rooms
}

func loadPrevSuumo() *Suumo {
	bytes, e := os.ReadFile(tmpFilePath())
	if e != nil {
		postToSlack(e.Error())
		log.Fatal(e.Error())
	}
	var prevData *Suumo
	if e := json.Unmarshal(bytes, &prevData); e != nil {
		postToSlack(e.Error())
		log.Fatal(e.Error())
	}
	return prevData
}

func genApartmentMessages(apts []*Apartment) []string {
	var aptMsgs []string
	for i := 0; i < len(apts); i++ {
		aptMsgs = append(aptMsgs, genApartmentDescription(apts[i]))
	}
	return aptMsgs
}

func genApartmentDescription(apt *Apartment) string {
	var roomDescs []string
	for i := 0; i < len(apt.Rooms); i++ {
		r := apt.Rooms[i]
		rDesc := genRoomDescription(r)
		roomDescs = append(roomDescs, rDesc)
	}
	rDesc := strings.Join(roomDescs, "\n     ----------\n")
	roomDescs = nil
	res := fmt.Sprintf(`物件名: %s
所在地: %s
最寄り: %s
築年: %s
階建: %s
部屋:
%s`, apt.Name, apt.Address, strings.Join(apt.Transports, ", "), apt.AgeOfBuilding, apt.TotalFloor, rDesc)
	return res
}

func genRoomDescription(r *Room) string {
	rows := []string{
		// fmt.Sprintf(`    - サムネ: %s`, r.Thumbnail), // data uri が入ってくる
		fmt.Sprintf(`    - 階: %s`, r.Floor),
		fmt.Sprintf(`    - 間取り: %s`, r.Madori),
		fmt.Sprintf(`    - 面積: %s`, r.Menseki),
		fmt.Sprintf(`    - 家賃: %s`, r.RentPrice),
		fmt.Sprintf(`    - 管理費: %s`, r.AdminPrice),
		fmt.Sprintf(`    - 礼金: %s`, r.GratuityPrice),
		fmt.Sprintf(`    - 敷金: %s`, r.DepositPrice),
		fmt.Sprintf(`    - URL: %s`, r.Url),
	}
	return strings.Join(rows, "\n")
}

func extractDiff(crntSm *Suumo, prevSm *Suumo) []*Apartment {
	var res []*Apartment

	eqHit := crntSm.Hit == prevSm.Hit
	eqCondArea := crntSm.CondArea == prevSm.CondArea
	eqTotalPage := crntSm.TotalPage == prevSm.TotalPage
	if eqHit && eqCondArea && eqTotalPage {
		return res
	}

	res = filterNewestApartments(crntSm.Apartments, prevSm.Apartments)
	return res
}

func filterNewestApartments(crntAps []*Apartment, prevAps []*Apartment) []*Apartment {
	var apts []*Apartment
	for i := 0; i < len(crntAps); i++ {
		cAp := crntAps[i]
		if findApartmentIndex(prevAps, cAp) == -1 {
			apts = append(apts, cAp)
		}
	}
	return apts
}

func findApartmentIndex(aps []*Apartment, needle *Apartment) int {
	index := -1
	for i := 0; i < len(aps); i++ {
		found := aps[i].Name == needle.Name
		if found {
			index = i
		}
	}
	return index
}

func getMaxPageNum(s *goquery.Selection) int {
	i, e := strconv.Atoi(s.Find(".pagination-parts > li").Last().Text())
	if e != nil {
		log.Fatal(e.Error())
	}
	return i
}

func tmpDirPath() string {
	user, err := user.Current()
	if err != nil {
		log.Fatal(err.Error())
	}
	baseDir := user.HomeDir
	return filepath.Join(baseDir, TMP_DIR_NAME)
}

func tmpFilePath() string {
	return filepath.Join(tmpDirPath(), OLD_TMP_FILE_NAME)
}

func trim(str string) string {
	return strings.TrimSpace(str)
}
