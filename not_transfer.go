package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/tidwall/gjson"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
)

const ANTON_TX = "https://anton.tools/api/v0/transactions?hash="
const NOT_MASTER = "EQAvlWFDxGF2lXm67y4yzC17wYKD9A0guwPkMs1gOsM__NOT"

type Transfer struct {
	From       *address.Address `json:"from"`
	To         *address.Address `json:"to"`
	Value      *big.Int         `json:"value"`
	ValueHuman string           `json:"value_human"`
	Comment    string           `json:"comment"`
}

type NotTransfer struct {
	messageChan     chan *Message
	bigTransferChan chan *Transfer
	howBig          tlb.Coins
}

func NewNotTransfer(howBig tlb.Coins) *NotTransfer {
	return &NotTransfer{
		howBig:          howBig,
		messageChan:     make(chan *Message, 10),
		bigTransferChan: make(chan *Transfer, 10),
	}
}

func tryGet(url string, times uint, interval time.Duration) (string, error) {
	for i := uint(0); i < times; i++ {
		resp, err := http.Get(url)
		if err != nil {
			return "", err
		}

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		total := gjson.Get(string(body), "total").Int()
		if total == 0 {
			time.Sleep(interval)
			continue
		}

		return string(body), nil
	}

	return "", fmt.Errorf("Failed to get %s", url)
}

func (n *NotTransfer) getTransferInfo(hash string) {
	body, err := tryGet(fmt.Sprintf("%s%s", ANTON_TX, hash), 3, 10*time.Second)
	if err != nil {
		log.Err(err).Msg("Failed to get transaction info")
		return
	}

	minterAddress := gjson.Get(string(body), "results.0.account.minter_address.base64").String()
	if minterAddress != NOT_MASTER {
		return
	}

	var t Transfer
	fmt.Println("1", gjson.Get(string(body), "results.0.account.address.base64").String())
	fmt.Println("2", gjson.Get(string(body), "results.0.in_msg.data.destination").String())
	t.From, err = address.ParseAddr(gjson.Get(string(body), "results.0.account.address.base64").String())
	if err != nil {
		log.Err(err).Msg("Failed to parse from address")
		return
	}

	t.To, err = address.ParseAddr(gjson.Get(string(body), "results.0.in_msg.data.destination").String())
	if err != nil {
		log.Err(err).Msg("Failed to parse to address")
		return
	}

	var ok bool
	t.Value, ok = new(big.Int).SetString(gjson.Get(string(body), "results.0.in_msg.data.amount").String(), 10)
	if !ok {
		log.Err(err).Msg("Failed to parse value")
		return
	}

	t.ValueHuman = tlb.FromNanoTON(t.Value).String()

	fmt.Println(t)

	n.bigTransferChan <- &t

}

func (t *NotTransfer) eventsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Type")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	for {
		select {
		case t := <-t.bigTransferChan:
			json.NewEncoder(w).Encode(t)
			w.(http.Flusher).Flush()
		}
	}
}

func (n *NotTransfer) Run() {
	go func() {
		http.HandleFunc("/events", n.eventsHandler)
		http.ListenAndServe(":8080", nil)
	}()

	for {
		select {
		case m := <-n.messageChan:
			go n.getTransferInfo(m.Hash)
		}
	}
}
