package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/m-m-f/gowiki"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

const name = "nostr-whatstoday"

const version = "0.0.1"

var revision = "HEAD"

var relays = []string{
	"wss://relay-jp.nostr.wirednet.jp",
	"wss://yabu.me",
	"wss://relay.nostr.band",
	"wss://nos.lol",
}

type payload struct {
	Batchcomplete bool `json:"batchcomplete"`
	Query         struct {
		Pages []struct {
			Ns        int64 `json:"ns"`
			Pageid    int64 `json:"pageid"`
			Revisions []struct {
				Content       string `json:"content"`
				Contentformat string `json:"contentformat"`
				Contentmodel  string `json:"contentmodel"`
			} `json:"revisions"`
			Title string `json:"title"`
		} `json:"pages"`
	} `json:"query"`
	Warnings struct {
		Main struct {
			Warnings string `json:"warnings"`
		} `json:"main"`
		Revisions struct {
			Warnings string `json:"warnings"`
		} `json:"revisions"`
	} `json:"warnings"`
}

func postNostr(nsec string, rs []string, content string) error {
	ev := nostr.Event{}
	var sk string
	if _, s, err := nip19.Decode(nsec); err != nil {
		return err
	} else {
		sk = s.(string)
	}
	if pub, err := nostr.GetPublicKey(sk); err == nil {
		if _, err := nip19.EncodePublicKey(pub); err != nil {
			return err
		}
		ev.PubKey = pub
	} else {
		return err
	}
	ev.Content = content
	ev.CreatedAt = nostr.Now()
	ev.Kind = nostr.KindTextNote
	ev.Tags = nostr.Tags{}
	ev.Tags = ev.Tags.AppendUnique(nostr.Tag{"t", "今日は何の日"})
	ev.Sign(sk)

	success := 0
	ctx := context.Background()
	for _, r := range rs {
		relay, err := nostr.RelayConnect(context.Background(), r)
		if err != nil {
			log.Printf("%v: %v", r, err)
			continue
		}
		err = relay.Publish(ctx, ev)
		relay.Close()
		if err == nil {
			success++
		}
	}
	if success == 0 {
		return errors.New("failed to publish")
	}
	return nil
}

func main() {
	var ver bool
	flag.BoolVar(&ver, "v", false, "show version")
	flag.Parse()

	if ver {
		fmt.Println(version)
		os.Exit(0)
	}

	date := time.Now().Format("1月2日")
	resp, err := http.Get(`https://ja.wikipedia.org/w/api.php?format=json&action=query&prop=revisions&rvprop=content&formatversion=2&titles=` + url.QueryEscape(date))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	var p payload
	err = json.NewDecoder(resp.Body).Decode(&p)
	if err != nil {
		log.Fatal(err)
	}

	content := p.Query.Pages[0].Revisions[0].Content

	begin := "\n== 記念日・年中行事 ==\n"
	pos := strings.Index(content, begin)
	if pos < 0 {
		log.Fatal("invalid")
	}
	content = content[pos+len(begin):]
	pos = strings.Index(content, "\n==")
	if pos < 0 {
		log.Fatal("invalid")
	}
	content = content[:pos]

	var buf bytes.Buffer
	fmt.Fprintln(&buf, date+"は")
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "*") {
			continue
		}
		if strings.HasPrefix(line, "*:") {
			continue
		}

		article, err := gowiki.ParseArticle("foo", line[1:], &gowiki.DummyPageGetter{})
		if err != nil {
			log.Fatal(err)
		}
		text := strings.TrimSpace(article.GetText())
		pos = strings.Index(text, "（）")
		if pos >= 0 {
			text = strings.TrimSpace(text[:pos])
		}
		if len(text) == 0 {
			continue
		}
		fmt.Fprintln(&buf, "* "+text)
	}
	fmt.Fprintln(&buf, "#今日は何の日")

	postNostr(os.Getenv("BOT_NSEC"), relays, buf.String())
}
