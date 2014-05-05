package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bitly/go-nsq"
	"github.com/joshuarubin/chanpubsub"
)

const (
	numModules = 10
	lookupd    = "127.0.0.1:4161" // this addr is OK to hardcode
	nsqd       = "127.0.0.1:4150" // this addr is OK to hardcode
	nsqhttpd   = "127.0.0.1:4151" // this addr is OK to hardcode
	nsqChan    = "worker"
)

type msgHandler struct {
	ID     int
	SigPub *chanpubsub.ChanPubSub
}

func (h *msgHandler) HandleMessage(message *nsq.Message) error {
	if h.ID+1 < numModules {
		var bodyBuf bytes.Buffer
		bodyBuf.Write(message.Body)
		fmt.Fprintf(&bodyBuf, "processed by mod%d\n", h.ID)
		topic := fmt.Sprintf("mod%d", h.ID+1)
		//log.Printf("publishing to topic %s\n", topic)
		if _, _, err := nsq.NewWriter(nsqd).Publish(topic, bodyBuf.Bytes()); err != nil {
			log.Println(err)
			return err
		}
	} else if h.ID+1 == numModules {
		log.Printf("received on mod%d\n%s\n", h.ID, message.Body)
		h.SigPub.Pub(struct{}{})
	}

	return nil
}

func module(moduleID int, ps *chanpubsub.ChanPubSub, termChan <-chan interface{}, exitChan chan<- struct{}) {
	topic := fmt.Sprintf("mod%d", moduleID)
	createTopic(topic)

	go func() {
		//log.Printf("topic: %s, channel: %s\n", topic, nsqChan)

		reader, err := nsq.NewReader(topic, nsqChan)
		if err != nil {
			log.Fatalln(err)
		}

		reader.VerboseLogging = false
		reader.LookupdPollInterval = 15 * time.Second

		reader.AddHandler(&msgHandler{
			ID:     moduleID,
			SigPub: ps,
		})

		err = reader.ConnectToLookupd(lookupd)
		if err != nil {
			log.Fatalln(err)
		}

		for {
			select {
			case <-reader.ExitChan:
				exitChan <- struct{}{}
				return
			case <-termChan:
				//log.Printf("module %d received signal\n", moduleID)
				reader.Stop()
			}
		}
	}()
}

func sigHandler() *chanpubsub.ChanPubSub {
	termChan := make(chan os.Signal, 1)
	signal.Notify(termChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	ps := chanpubsub.New()

	go func() {
		ps.Pub(<-termChan)
		return
	}()

	return ps
}

func createTopic(topic string) {
	if _, err := http.Get("http://" + nsqhttpd + "/create_topic?topic=" + topic); err != nil {
		log.Fatal(err)
	}
}

func main() {
	ps := sigHandler()
	exitChan := make(chan struct{})

	for i := 0; i < numModules; i++ {
		moduleID := i // get a loop local counter
		termChan, _ := ps.Sub()
		module(moduleID, ps, termChan, exitChan)
	}

	if _, _, err := nsq.NewWriter(nsqd).Publish("mod0", []byte("sent\n")); err != nil {
		log.Fatalln(err)
	}
	//log.Println("message sent")

	for i := 0; i < numModules; i++ {
		<-exitChan
		//log.Println("module exit", i)
	}
}
