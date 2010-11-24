package main

import (
	"http"
	"flag"
	"git.78762.de/go/error"
	"container/list"
	"regexp"
	"strings"
	"bufio"
	"bytes"
	"io"
	"runtime"
)

var (
	lang          string
	rUnneededTags *regexp.Regexp
	rAllTags      *regexp.Regexp
	rATags        *regexp.Regexp
	rAlphaNum     *regexp.Regexp
	visitedLinks  map[string]bool
)

type WikiArticle struct {
	url     string
	depth   int
	content string
}

func InitGlobals() {
	rUnneededTags = regexp.MustCompile("<[^a][^>]*>")
	rAllTags = regexp.MustCompile("<[^>]+>")
	rATags = regexp.MustCompile("<a[^>]*href=\"(/wiki/[^?\"]+)\"[^>]*>")
	rAlphaNum = regexp.MustCompile("[a-zA-Z0-9,.\\-]+")

	visitedLinks = make(map[string]bool)
}

func main() {
	error.SetPrefix("WikiCrawl")
	error.PrintBacktrace(true)
	defer error.ErrorHandler()

	InitGlobals()

	startpage := flag.String("s", "/wiki/Special:Random", "Page to start crawling at")
	flag.StringVar(&lang, "l", "en", "Language of the wiki pages")
	numlinks := flag.Int("n", 500, "Number of links to investigate")
	flag.Parse()

	queue := list.New()
	queue.PushBack(&WikiArticle{url: "http://" + lang + ".wikipedia.org" + *startpage, depth: 0})

	lines := Crawl(queue, *numlinks)
	words := Wordify(lines)
	_ = words
	for _ = range words { }
	//wordlist := CreateWordlist(lines)

}

func Crawl(queue *list.List, limit int) chan string {
	articles := Fetcher(queue, limit)
	lines, links := ContentExtractor(articles)
	PushLinks(queue, links)
	return lines
}

func Fetcher(queue *list.List, limit int) chan *WikiArticle {
	articles := make(chan *WikiArticle)
	go func() {
		for i := 0; i < limit; i++ {
			for queue.Front() == nil {
				runtime.Gosched()
			}
			// url := q.PopFront()
			article := queue.Front().Value.(*WikiArticle)
			queue.Remove(queue.Front())

			r, url, e := http.Get(article.url)
			if e != nil {
				println("Failed:", article.url)
				continue
			}
			println("Fetched:", article.depth, url)

			buf := bytes.NewBufferString("")
			io.Copy(buf, r.Body)
			article.content = buf.String()
			r.Body.Close()
			articles <- article
		}
		close(articles)
	}()
	return articles
}

func ContentExtractor(articles chan *WikiArticle) (chan string, chan *WikiArticle) {
	text := make(chan string)
	linkedArticles := make(chan *WikiArticle)
	go func() {
		for article := range articles {
			article.content = truncateToBodyContent(article.content)
			article.content = removeUnneededTags(article.content)
			text <- extractText(article.content)
			extractLinkedArticles(linkedArticles, article)
		}
		close(text)
		close(linkedArticles)
	}()
	return text, linkedArticles
}

func PushLinks(queue *list.List, linkedArticles chan *WikiArticle) {
	go func() {
		for article := range linkedArticles {
			article.url = "http://" + lang + ".wikipedia.org" + article.url
			queue.PushBack(article)
		}
	}()
}

func truncateToBodyContent(content string) string {
	s := ""
	br := bufio.NewReader(strings.NewReader(content))
	line, e := br.ReadString('\n')
	foundStartSignal := false
	for e == nil {
		if !foundStartSignal {
			foundStartSignal = containsStartSignal(line)
		} else if !containsStopSignal(line) {
			s += line
		} else {
			break
		}
		line, e = br.ReadString('\n')
	}
	return s
}
func containsStartSignal(line string) bool {
	return strings.Contains(line, "<!-- bodyContent -->")
}

func containsStopSignal(line string) bool {
	return strings.Contains(line, "<!-- /bodyContent -->")
}

func removeUnneededTags(s string) (r string) {
	return rUnneededTags.ReplaceAllString(s, "")

}

func extractText(s string) string {
	return rAllTags.ReplaceAllString(s, "")
}

func extractLinkedArticles(linkedArticles chan *WikiArticle, article *WikiArticle) {
	matches := rATags.FindAllStringSubmatchIndex(article.content, -1)
	if matches == nil {
		return
	}

	for _, match := range matches {
		rawmatch := article.content[match[2]:match[3]]
		if isUnwantedPage(rawmatch) {
			continue
		}
		_, ok := visitedLinks[rawmatch]
		if ok {
			continue
		}
		visitedLinks[rawmatch] = true
		linkedArticles <- &WikiArticle{url: rawmatch, depth: article.depth + 1}
	}
	return
}

func Wordify(lines chan string) chan string {
	words := make(chan string)
	go func() {
		for line := range lines {
			br := bufio.NewReader(strings.NewReader(line))
			w, e := br.ReadString(' ')
			for e == nil {
				if rAlphaNum.MatchString(w) {
					words <- strings.TrimSpace(w)
				}
				w, e = br.ReadString(' ')
			}
		}
		close(words)
	}()
	return words
}

func isUnwantedPage(path string) bool {
	prefixes := []string{"File", "Special", "Wikipedia", "Template", "Talk", "Help"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, "/wiki/"+prefix+":") {
			return true
		}
	}
	return false
}
