package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fluffle/goirc/client"
	"github.com/fluffle/goirc/logging"
	"github.com/generaltso/tsobot/dongers"
)

/**
 * Configuration variables, passed in with command line flags
 */
var host string
var port int
var ssl bool
var nick string
var pass string
var join string
var admin string
var cache_dir string

/**
 * Arbitrary way GoIRC handles logging
 */
type tsoLogger struct{}

func (l *tsoLogger) Debug(f string, a ...interface{}) { log.Printf(f+"\n", a...) }
func (l *tsoLogger) Info(f string, a ...interface{})  { log.Printf(f+"\n", a...) }
func (l *tsoLogger) Warn(f string, a ...interface{})  { log.Printf(f+"\n", a...) }
func (l *tsoLogger) Error(f string, a ...interface{}) { log.Printf(f+"\n", a...) }

/**
 * More boilerplate
 */
func checkErr(err error) {
	if err != nil {
		log.Panicln(err)
	}
}

/**
 * Botnet
 */
var botAdmins sort.StringSlice
var botCommandRe *regexp.Regexp = regexp.MustCompile(`^\.(\w+)\s*(.*)$`)

type botCommand struct {
	admin bool
	fn    func(who, arg, nick string)
}

var botCommands map[string]*botCommand
var sendMessage func(who, msg string)

func parseMessage(who, msg, nick string) bool {
	if !botCommandRe.MatchString(msg) {
		return false
	}

	m := botCommandRe.FindStringSubmatch(msg)
	cmd := m[1]
	arg := m[2]

	if b, ok := botCommands[cmd]; ok {
		if !b.admin || (b.admin && isAdmin(nick)) {
			b.fn(who, arg, nick)
		} else {
			//log.Printf("%#v\n", botAdmins)
			sendMessage(nick, "Access denied.")
		}
		return true
	}
	return false
}

func isAdmin(nick string) bool {
	ind := sort.SearchStrings(botAdmins, nick)
	return botAdmins[ind] == nick
}

func addAdmin(nick string) {
	botAdmins = append(botAdmins, nick)
	botAdmins = sort.StringSlice(botAdmins)
}

func removeAdmin(nick string) {
	ind := sort.SearchStrings(botAdmins, nick)
	if botAdmins[ind] == nick {
		botAdmins = append(botAdmins[:ind], botAdmins[ind+1:]...)
		botAdmins = sort.StringSlice(botAdmins)
	}
}

func main() {
	flag.StringVar(&host, "host", "irc.rizon.net", "host")
	flag.IntVar(&port, "port", 6697, "port")
	flag.BoolVar(&ssl, "ssl", true, "use ssl?")

	flag.StringVar(&nick, "nick", "tsobot", "nick")
	flag.StringVar(&pass, "pass", "", "NickServ IDENTIFY password (optional)")
	flag.StringVar(&join, "join", "tso", "space separated list of channels to join")

	flag.StringVar(&admin, "admin", "tso", "space separated list of privileged nicks")
	flag.StringVar(&cache_dir, "cache_dir", ".cache", "directory to cache datas like rss feeds")

	flag.Parse()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	signal.Notify(sig, os.Kill)

	logging.SetLogger(&tsoLogger{})

	cfg := client.NewConfig(nick)
	if ssl {
		cfg.SSL = true
		cfg.SSLConfig = &tls.Config{ServerName: host, InsecureSkipVerify: true}
		cfg.NewNick = func(n string) string { return n + "^" }
	}
	cfg.Server = fmt.Sprintf("%s:%d", host, port)
	irc := client.Client(cfg)

	irc.HandleFunc(client.CONNECTED, func(c *client.Conn, l *client.Line) {
		if pass != "" {
			irc.Privmsg("NickServ", "IDENTIFY "+pass)
		}
		for _, ch := range strings.Split(join, " ") {
			irc.Join("#" + ch)
		}
		botAdmins = sort.StringSlice(strings.Split(admin, " "))
	})

	ded := make(chan struct{})
	irc.HandleFunc(client.DISCONNECTED, func(c *client.Conn, l *client.Line) {
		close(ded)
	})

	sendMessage = func(who, msg string) {
		irc.Privmsg(who, msg)
	}

	cache = make(chan *clickbait)
	noiz = make(chan *clickbait)
	chat = make(chan *line)
	go cacheHandler()

	chanRe := regexp.MustCompile(`(?i)^\#[^\s]+$`)
	nickRe := regexp.MustCompile(`(?i)^[a-z_\-\[\]\\^{}|` + "`" + `][a-z0-9_\-\[\]\\^{}|` + "`" + `]*$`)
	urlRe := regexp.MustCompile(`(?i)^https?://[^\s]+$`)

	botCommands = map[string]*botCommand{
		"bots": &botCommand{false, func(who, arg, nick string) {
			sendMessage(who, "Reporting in! "+colorString("go", White, Black)+" get github.com/generaltso/tsobot")
		}},
		"add_admin": &botCommand{true, func(who, arg, nick string) {
			for _, adm := range strings.Split(arg, " ") {
				irc.Whois(adm)
			}
		}},
		"remove_admin": &botCommand{true, func(who, arg, nick string) {
			for _, adm := range strings.Split(arg, " ") {
				if isAdmin(adm) {
					removeAdmin(adm)
					sendMessage(adm, "see you space cowboy...")
				}
			}
		}},
		"join": &botCommand{true, func(who, arg, nick string) {
			for _, ch := range strings.Split(arg, " ") {
				irc.Join(ch)
			}
		}},
		"part": &botCommand{true, func(who, arg, nick string) {
			irc.Part(who, arg)
		}},
		"tone_police": &botCommand{false, func(who, arg, nick string) {
			arg = strings.TrimSpace(arg)
			var input []byte
			var debug_input string
			switch {
			case arg == "":
				input = getLines("nick", nick)
				debug_input = "nick " + nick
			case nickRe.MatchString(arg):
				input = getLines("nick", arg)
				debug_input = "nick " + arg
			case chanRe.MatchString(arg):
				input = getLines("chan", arg)
				debug_input = "channel " + arg
			case urlRe.MatchString(arg):
				text, err := scrape(arg)
				if err != nil {
					log.Println("__ERROR", err)
					sendMessage(who, err.Error())
					return
				}
				input = []byte(text)
			default:
				input = []byte(arg)
				debug_input = "...oh wait no you put in a sentence " + dongers.Raise("Panic")
			}
			if len(input) == 0 {
				sendMessage(who, dongers.Raise("Sadness")+" No lines in buffer for "+debug_input)
				return
			}

			log.Println("__DEBUG", debug_input, ":", string(input))

			tone := tonePolice(input)

			// temporary, need to change API...
			if tone.Neutral == 1 {
				tone.Neutral = 0
			}

			emote, score := tone.Max()

			var response string
			if score == 0 {
				response = dongers.Raise("Panic") + " (no information in result set)"
			} else {
				response = fmt.Sprintf("%s: %.2f%% %s", emote, score*100.0, dongers.Raise(emote))
			}
			sendMessage(who, response)
			irc.Notice(nick, fmt.Sprintf("anger %.2f disgust %.2f fear %.2f happy %.2f neutral %.2f sad %.2f surprise %.2f", tone.Anger, tone.Disgust, tone.Fear, tone.Happiness, tone.Neutral, tone.Sadness, tone.Surprise))
		}},
		"add_rss": &botCommand{true, func(who, arg, nick string) {
			if strings.TrimSpace(arg) == "" {
				sendMessage(who, "usage: .add_rss [URL]")
				return
			}
			subs = append(subs, &subscription{who: who, src: arg})
			log.Printf("\n\nsubs:%#v\n\n", subs)
			go pollFeed(arg)
			sendMessage(who, "Subscribed "+who+" to "+arg)
		}},
		"trans": &botCommand{false, func(who, arg, nick string) {
			arg = strings.Replace(arg, "/", "", -1)
			sendMessage(who, translate(arg))
		}},
	}
	irc.HandleFunc("307", func(c *client.Conn, l *client.Line) {
		if l.Args[0] == nick {
			addAdmin(l.Args[1])
			sendMessage(l.Args[1], "you know what you doing")
		}
		//log.Println("\n\n---\ngot auth !!\n")
		//log.Printf("%#v %#v\n", c, l)
	})
	//irc.HandleFunc("318", func(c *client.Conn, l *client.Line) {
	//log.Println("\n\n---\ngot end of whois\n\n")
	//log.Printf("%#v %#v\n", c, l)
	//})
	irc.HandleFunc(client.PRIVMSG, func(c *client.Conn, l *client.Line) {
		//log.Printf("%#v\n", l)
		who, msg := l.Args[0], l.Args[1]
		if who == nick {
			who = l.Nick
		}
		if !parseMessage(who, msg, l.Nick) {
			go logLine(l)
		}

	})

	if err := irc.ConnectTo(host); err != nil {
		log.Fatalln("Connection error:", err)
	}

	for {
		select {
		case bait := <-noiz:
			log.Printf("\n\nbait:%#v\n\n", bait)
			for _, ch := range subs {
				if ch.src == bait.src {
					sendMessage(ch.who, fmt.Sprintf("%s — !%s", bait.tit, bait.url))
				}
			}
		case <-sig:
			log.Println("we get signal")
			for _, ch := range strings.Split(join, " ") {
				irc.Part("#"+ch, "we get signal")
			}
			<-time.After(time.Second)
			irc.Quit()
			os.Exit(0)
		case <-ded:
			log.Println("disconnected.")
			os.Exit(1)
		}
	}
}
