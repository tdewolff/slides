package main

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/matryer/try"
	"github.com/russross/blackfriday/v2"
	flag "github.com/spf13/pflag"
)

type Slides struct {
	Title   string
	Content []template.HTML
}

type Message struct {
	Type string
	Data interface{}
}

type Server struct {
	tpls *template.Template
}

func NewServer() *Server {
	tpls := template.Must(template.New("").Funcs(template.FuncMap{
		"add": func(i, j int) int {
			return i + j
		},
	}).ParseGlob("*.tpl"))

	return &Server{
		tpls: tpls,
	}
}

func (h *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/ws" {
		h.ServeWS(w, r)
		return
	}

	w.Header().Set("Content-Security-Policy", "script-src 'self'")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	filename := path.Clean(r.URL.Path)
	if len(filename) > 0 && filename[0] == '/' {
		filename = filename[1:]
	}

	if filename == "" {
		index := ""

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.tpls.ExecuteTemplate(w, "index.tpl", index); err != nil {
			log.Println(err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	} else if path.Ext(filename) == ".slide" {
		var b []byte
		err := try.Do(func(attempt int) (bool, error) {
			var err error
			b, err = ioutil.ReadFile(filename)
			return attempt < 5, err
		})
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			} else {
				log.Println(err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
			return
		}

		slides := parseSlides(b)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.tpls.ExecuteTemplate(w, "slides.tpl", slides); err != nil {
			log.Println(err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	} else if strings.HasPrefix(filename, "res/") {
		f, err := os.Open(filename)
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			} else {
				log.Println(err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
			return
		}
		defer f.Close()

		switch path.Ext(r.URL.Path) {
		case ".css":
			w.Header().Set("Content-Type", "text/css")
		case ".js":
			w.Header().Set("Content-Type", "application/javascript")
		case ".woff2":
			w.Header().Set("Content-Type", "font/woff2")
		}
		io.Copy(w, f)
	} else {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
}

func (h *Server) ServeWS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wg := sync.WaitGroup{}
	defer wg.Wait()

	if r.Header.Get("Origin") != "http://"+r.Host {
		http.Error(w, "Origin not allowed", http.StatusForbidden)
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("websocket:", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	defer conn.Close()
	conn.EnableWriteCompression(true)

	messages := make(chan Message, 5)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				close(messages)
				return
			}

			if messageType == websocket.TextMessage {
				var msg Message
				if err := json.Unmarshal(p, &msg); err != nil {
					log.Println("could not unmarshal JSON:", err)
					continue
				}
				messages <- msg
			}
		}
	}()

	watcher, err := NewWatcher(ctx)
	if err != nil {
		log.Println("watcher:", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	defer watcher.Close()

	watcher.Add("index.tpl")
	watcher.Add("slides.tpl")
	watcher.Add("res/style.css")
	watcher.Add("res/script.js")

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := watcher.Watch(); err != nil {
			log.Println("watcher:", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-messages:
			if !ok {
				// client disconnected
				return
			}

			switch msg.Type {
			case "error":
				if err, ok := msg.Data.(string); ok {
					log.Println("client:", err)
				} else {
					log.Println("could not parse message:", msg)
				}
			case "watch":
				if filename, ok := msg.Data.(string); ok {
					if len(filename) > 0 && filename[0] == '/' {
						filename = filename[1:]
					}
					watcher.Add(filename)
				} else {
					log.Println("could not parse message:", msg)
				}
			default:
				log.Println("unhandled message:", msg)
			}
		case _, ok := <-watcher.Changed():
			if !ok {
				watcher.Close()
				continue
			}

			msg, _ := json.Marshal(Message{
				Type: "refresh",
			})
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Println("WriteMessage:", err)
				continue
			}
		}
	}
}

func parseSlides(slides []byte) Slides {
	content := []template.HTML{}
	for _, slide := range bytes.Split(slides, []byte("---")) {
		slide = bytes.TrimSpace(slide)
		slide := blackfriday.Run(slide)
		content = append(content, template.HTML(slide))
	}

	return Slides{
		Title:   "Title",
		Content: content,
	}
}

func main() {
	var addr string
	flag.StringVar(&addr, "port", ":8080", "listening port")
	flag.Parse()

	h := NewServer()

	server := &http.Server{
		Addr:    addr,
		Handler: h,
	}

	done := make(chan struct{})
	interrupt := make(chan os.Signal)
	signal.Notify(interrupt, os.Interrupt)

	go func() {
		<-interrupt

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Fatalf("could not gracefully shutdown server: %v\n", err)
		}
		close(done)
	}()

	log.Printf("server is listening at http://localhost%s/\n", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("error: could not listen on %s: %v\n", addr, err)
	}
}
