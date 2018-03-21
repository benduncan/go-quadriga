package goquadriga

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const V2URL = "https://api.quadrigacx.com/v2/"

type Client struct {
	RootUrl   string
	ClientId  string
	ApiKey    string
	ApiSecret string
	Debug     bool
}

func NewClient(id string, key string, secret string, debug bool) *Client {
	return &Client{
		RootUrl:   V2URL,
		ClientId:  id,
		ApiKey:    key,
		ApiSecret: secret,
		Debug:     debug,
	}
}

func (c *Client) URL(res string) string {
	return fmt.Sprintf("%s%s", c.RootUrl, res)
}

func (c *Client) GetCurrentTradingInfo(book string) (CurrentTrade, error) {
	var current CurrentTrade

	// btc_cad is the default book type, allow btc_usd, etc as an argument
	var args string

	if book != "" {
		args = fmt.Sprintf("?book=%s", book)
	}

	body, err := c.get(c.URL("ticker" + args))

	if err != nil {
		return current, err
	}
	err = json.Unmarshal(body, &current)
	return current, err
}

func (c *Client) GetOrderBook() (OrderBook, error) {
	var orders OrderBook

	body, err := c.get(c.URL("order_book"))
	if err != nil {
		return orders, err
	}
	err = json.Unmarshal(body, &orders)
	return orders, err
}

func (c *Client) GetTransactions() (TransactionResponse, error) {
	var transactions TransactionResponse

	body, err := c.get(c.URL("transactions"))
	if err != nil {
		return transactions, err
	}
	err = json.Unmarshal(body, &transactions)
	fmt.Printf("Results: %v\n", transactions)
	return transactions, err
}

func (c *Client) PostAccountBalance() (AccountBalance, error) {
	var balance AccountBalance

	auth := c.makeSig()
	payload, err := json.Marshal(auth)
	if err != nil {
		return balance, err
	}
	fmt.Println("Payload =>", string(payload))

	body, err := c.post(c.URL("balance"), payload)

	fmt.Println("Body => ", string(body))

	fmt.Println("Err => ", err)

	if err != nil {
		return balance, err
	}
	err = json.Unmarshal(body, &balance)
	return balance, nil
}

func (c *Client) PostOpenOrders() (OpenOrdersResponse, error) {
	var orders OpenOrdersResponse

	auth := c.makeSig()
	payload, err := json.Marshal(auth)
	if err != nil {
		return orders, err
	}
	fmt.Println(string(payload))

	body, err := c.post(c.URL("open_orders"), payload)
	if err != nil {
		return orders, err
	}
	err = json.Unmarshal(body, &orders)
	return orders, nil
}

func (c *Client) PostOrderLookup(id string) (LookupOrderResponse, error) {
	var lookup OrderId
	var order []LookupOrderResponse
	lookup.ID = id

	auth := c.makeSig()
	payload, err := json.Marshal(struct {
		*BaseAuth
		OrderId
	}{auth, lookup})
	if err != nil {
		return order[0], err
	}
	fmt.Println(string(payload))

	body, err := c.post(c.URL("lookup_order"), payload)
	if err != nil {
		return order[0], err
	}
	err = json.Unmarshal(body, &order)
	return order[0], err
}

func (c *Client) PostCancelOrder(id string) (bool, error) {
	var cancel OrderId
	cancel.ID = id

	auth := c.makeSig()
	payload, err := json.Marshal(struct {
		*BaseAuth
		OrderId
	}{auth, cancel})
	if err != nil {
		return false, err
	}
	fmt.Println(string(payload))

	body, err := c.post(c.URL("cancel_order"), payload)
	if err != nil {
		return false, err
	}
	cancelSuccess, err := strconv.ParseBool(strings.Trim(string(body), "\""))
	if err != nil {
		return false, err
	}
	return cancelSuccess, err
}

// Post a BUY order at the market price ( returned by the ticker, `bid` price )
func (c *Client) PostBuyMarketOrder(amount float64, book string) (orderResp BuyMarketOrderResponse, err error) {
	var buyorder BuyMarketOrder

	buyorder.Amount = amount
	buyorder.Book = book

	auth := c.makeSig()
	payload, err := json.Marshal(struct {
		*BaseAuth
		BuyMarketOrder
	}{auth, buyorder})

	if err != nil {
		return orderResp, err
	}

	fmt.Println(string(payload))

	body, err := c.post(c.URL("buy"), payload)

	fmt.Println("PostBuyMarketOrder =>", body, "error =>", err)

	if err != nil {
		return orderResp, err
	}

	err = json.Unmarshal(body, &orderResp)

	// Temp
	orderResp.Amount = amount

	return orderResp, err
}

// Local functions
func (c *Client) get(urlapi string) (body []byte, err error) {

	// "Fake" responses from Quadriga for testing, vs posting a live trade.
	if c.Debug == true {
		var str string

		fmt.Println("get debug mode =>", urlapi)

		// Parse the URL to retrieve the pathname
		u, err := url.Parse(urlapi)

		if err != nil {
			return body, err
		}

		if u.Path == "/v2/ticker" {
			str = "{\"high\":\"12000.00\",\"last\":\"11905.50\",\"timestamp\":\"1521612778\",\"volume\":\"400.13676232\",\"vwap\":\"11529.43609206\",\"low\":\"11701.00\",\"ask\":\"11979.99\",\"bid\":\"11905.50\"}"
		} else if u.Path == "/v2/order_book" {
			str = "{\"timestamp\":\"1521612857\",\"bids\":[[\"11905.50\",\"0.09024660\"],[\"11905.00\",\"0.31609072\"],[\"11902.00\",\"0.00102796\"],[\"11780.01\",\"3.52077000\"],[\"11780.00\",\"0.25000000\"],[\"11770.00\",\"0.06847539\"],[\"11750.01\",\"35.20770001\"],[\"11750.00\",\"0.55967830\"],[\"11707.00\",\"0.08550000\"],[\"11703.03\",\"0.81600000\"],[\"11703.00\",[\"12498.00\",\"0.20000000\"],[\"12499.00\",\"0.39610642\"],[\"12499.99\",\"0.35055000\"],[\"12500.00\",\"1.55585603\"]]}"
		}

		body = []byte(str)

	} else {

		fmt.Println("get non-debug mode =>", urlapi)

		resp, err := http.Get(urlapi)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		body, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

	}

	return body, nil
}

func (c *Client) post(urlapi string, payload []byte) (body []byte, err error) {

	// "Fake" responses from Quadriga for testing, vs posting a live trade.
	if c.Debug == true {
		var str string

		fmt.Println("post debug mode", urlapi)

		// Parse the URL to retrieve the pathname
		u, err := url.Parse(urlapi)

		if err != nil {
			return body, err
		}

		if u.Path == "/v2/buy" {
			str = "{\"amount\":\"0.00200000\",\"book\":\"btc_usd\",\"datetime\":\"2018-03-19 11:12:00\",\"id\":\"kbcbc1o4gj0bedrc13jclstsw1wqy2b05yqsewzn0hhixy36zttvebx5j70ovlxz\",\"price\":\"0.00\",\"status\":\"0\",\"type\":\"0\"}"
		} else if u.Path == "/v2/balance" {
			str = "{\"btc_available\":\"0.00947078\",\"btc_reserved\":\"0.00000000\",\"btc_balance\":\"0.00947078\",\"bch_available\":\"0.00000000\",\"bch_reserved\":\"0.00000000\",\"bch_balance\":\"0.00000000\",\"btg_available\":\"0.00000000\",\"btg_reserved\":\"0.00000000\",\"btg_balance\":\"0.00000000\",\"eth_available\":\"0.00000000\",\"eth_reserved\":\"0.00000000\",\"eth_balance\":\"0.00000000\",\"ltc_available\":\"0.00000000\",\"ltc_reserved\":\"0.00000000\",\"ltc_balance\":\"0.00000000\",\"etc_available\":\"0.00000000\",\"etc_reserved\":\"0.00000000\",\"etc_balance\":\"0.00000000\",\"cad_available\":\"0.00\",\"cad_reserved\":\"0.00\",\"cad_balance\":\"0.00\",\"usd_available\":\"486.42\",\"usd_reserved\":\"0.00\",\"usd_balance\":\"486.42\",\"xau_available\":\"0.000000\",\"xau_reserved\":\"0.000000\",\"xau_balance\":\"0.000000\",\"fee\":\"0.5000\",\"fees\":{\"btc_cad\":\"0.5000\",\"btc_usd\":\"0.5000\",\"eth_cad\":\"0.5000\",\"eth_btc\":\"0.2000\",\"ltc_cad\":\"0.5000\",\"ltc_btc\":\"0.2000\",\"bch_cad\":\"0.5000\",\"bch_btc\":\"0.2000\",\"btg_cad\":\"0.5000\",\"btg_btc\":\"0.2000\"}}"
		}

		body = []byte(str)

		fmt.Println("Debug resp =>", str)

	} else {

		fmt.Println("post non-debug mode")

		req, err := http.NewRequest("POST", urlapi, bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		fmt.Println("Response status: ", resp.Status)
		fmt.Println("Response headers: ", resp.Header)
		fmt.Println("Response body: ", string(body))

	}

	// Check if an error was returned
	var jsonError ErrorResponse

	err = json.Unmarshal(body, &jsonError)

	fmt.Println("Unmarshal jsonError =>", jsonError)

	if jsonError.Error.Message != "" {
		err = errors.New(jsonError.Error.Message)
	}

	return body, err
}

func (c *Client) makeSig() *BaseAuth {
	timestamp := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)

	message := strings.Join([]string{timestamp, c.ClientId, c.ApiKey}, "")
	fmt.Println("The message is ", message)

	sig := hmac.New(sha256.New, []byte(c.ApiSecret))
	sig.Write([]byte(message))

	base := &BaseAuth{ApiKey: c.ApiKey, Signature: hex.EncodeToString(sig.Sum(nil)), Nonce: timestamp}
	return base
}
