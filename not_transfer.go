package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"time"
	"strconv"
	"bytes"
	"strings"
    "encoding/base64"
    "encoding/hex"


	"github.com/rs/zerolog/log"
	"github.com/tidwall/gjson"
	// "github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
)

const ANTON_TX = "https://anton.tools/api/v0/transactions?hash="
// const NOT_MASTER = "EQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAM9c"
const NOT_MASTER = "EQAvlWFDxGF2lXm67y4yzC17wYKD9A0guwPkMs1gOsM__NOT"
const TONAPI_TOKEN = "AGPYBM7LJHSAI6AAAAACCHMNKXOAW6QRNZYLW7CSV57BX62JWJKY7B7FTWRRUR54MWMCKDQ"

type Transfer struct {
	From       string `json:"from"`
	To         string `json:"to"`
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
		return
	}

	minterAddress := gjson.Get(string(body), "results.0.account.minter_address.base64").String()
	if minterAddress != NOT_MASTER {
		return
	}

	var t Transfer
	fmt.Println("1", gjson.Get(string(body), "results.0.account.address.base64").String())
	fmt.Println("2", gjson.Get(string(body), "results.0.in_msg.data.destination").String())
	fromAddress, err := addressToAccount(gjson.Get(string(body), "results.0.account.address.base64").String())
	if err != nil {
		return
	}
	t.From = fromAddress

	toAddress, err := addressToAccount(gjson.Get(string(body), "results.0.in_msg.data.destination").String())
	if err != nil {
		return
	}
	t.To = toAddress

	var ok bool
	t.Value, ok = new(big.Int).SetString(gjson.Get(string(body), "results.0.in_msg.data.amount").String(), 10)
	if !ok {
		return
	}

	
	t.ValueHuman = tlb.FromNanoTON(t.Value).String()
	fmt.Println("Value human print>>", t.ValueHuman)

	// fmt.Println(t)
	valueHumanInt, err := strconv.ParseFloat(t.ValueHuman, 64)
	if err != nil {
		log.Err(err).Msg("Failed to convert ValueHuman to integer")
		return
	}

	if valueHumanInt > 1000000 {
		n.notifyBigTransfer(t)
	}

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


func (t *NotTransfer) notifyBigTransfer(transfer Transfer) {
	content := fmt.Sprintf("发送: %s\n接收: %s\n数目: %s NOT", transfer.From, transfer.To, transfer.ValueHuman)
	payload := map[string]string{"content": content}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("Error marshalling payload:", err)
		return
	}
	
	resp, err := http.Post("https://signarl.com/api/transaction/send_tele", "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		fmt.Println("Failed to send request, status code:", resp.StatusCode)
		return
	}
	
	fmt.Println("Successfully notified big transfer:", content)
}



func addressToAccount(addr string) (string, error) {
	newAdd := getAccoutbyWalletID(addr)
	fmt.Printf("newAdd>>",newAdd)
	url := fmt.Sprintf("https://tonapi.io/v2/accounts/0:%s", newAdd)
	fmt.Printf("url>>",url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+ TONAPI_TOKEN)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get account info, status code: %d", resp.StatusCode)
	}
	fmt.Printf(" resp.StatusCode  >> ", resp.StatusCode )


	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	if name, ok := result["name"].(string); ok {
		fmt.Printf("name >> ",name)
		return name, nil
	}
	return addr, nil
}


func decodeBase64URL(s string) ([]byte, error) {
    // Base64URL does not use padding, so we need to add it if necessary
    switch len(s) % 4 {
    case 2:
        s += "=="
    case 3:
        s += "="
    }
    return base64.URLEncoding.DecodeString(s)
}

func getAccoutbyWalletID(address string) (ret string) {
    // TON wallet address (base64url encoded)
    // address := "EQDRKHVsAVY49-TdN0Y5JkmpwqiuewMoLJPQMxle3QqgXkoL"

    // Decode the base64url encoded address
    decoded, err := decodeBase64URL(address)
    if err != nil {
        //log.Fatal(err)
    }

    // Ensure the decoded length is correct (1 byte for tag + 1 byte for workchain + 32 bytes for account ID + 2 checksum bytes)
    if len(decoded) != 36 {
        //log.Fatalf("Invalid address length: expected 36, got %d", len(decoded))
    }

    // Extract the workchain ID and account ID
    workchainID := int8(decoded[1]) // Second byte is the workchain ID
    accountID := decoded[2:34]      // Next 32 bytes are the account ID

    // Convert account ID to hexadecimal string
    accountIDHex := hex.EncodeToString(accountID)

    // Print the Account ID in the format <workchain>:<account_id>
    fmt.Printf("%d:%s\n", workchainID, strings.ToLower(accountIDHex))
    return strings.ToLower(accountIDHex)
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
