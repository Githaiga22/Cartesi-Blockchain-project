package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

var (
	rollupServer = os.Getenv("ROLLUP_HTTP_SERVER_URL")
	users        []string
	toUpperTotal int
	mu           sync.Mutex
)

func hex2str(hexStr string) (string, error) {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func str2hex(payload string) string {
	return hex.EncodeToString([]byte(payload))
}

func isNumeric(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func handleAdvance(data map[string]interface{}) (string, error) {
	log.Println("Received advance request data:", data)

	metadata := data["metadata"].(map[string]interface{})
	sender := metadata["msg_sender"].(string)
	payload := data["payload"].(string)

	sentence, err := hex2str(payload)
	if err != nil {
		return "reject", err
	}

	if isNumeric(sentence) {
		err := report("sentence is not in hex format")
		if err != nil {
			return "reject", err
		}
		return "reject", nil
	}

	mu.Lock()
	users = append(users, sender)
	toUpperTotal++
	mu.Unlock()

	sentence = strings.ToUpper(sentence)
	err = notice(sentence)
	if err != nil {
		return "reject", err
	}

	return "accept", nil
}

func handleInspect(data map[string]interface{}) (string, error) {
	log.Println("Received inspect request data:", data)

	payload := data["payload"].(string)
	route, err := hex2str(payload)
	if err != nil {
		return "reject", err
	}

	var responseObject string
	switch route {
	case "list":
		userData, _ := json.Marshal(map[string]interface{}{"users": users})
		responseObject = string(userData)
	case "total":
		totalData, _ := json.Marshal(map[string]interface{}{"toUpperTotal": toUpperTotal})
		responseObject = string(totalData)
	default:
		responseObject = "route not implemented"
	}

	err = report(responseObject)
	if err != nil {
		return "reject", err
	}

	return "accept", nil
}

func report(message string) error {
	payload := str2hex(message)
	data, _ := json.Marshal(map[string]string{"payload": payload})
	_, err := http.Post(rollupServer+"/report", "application/json", bytes.NewBuffer(data))
	return err
}

func notice(message string) error {
	payload := str2hex(message)
	data, _ := json.Marshal(map[string]string{"payload": payload})
	_, err := http.Post(rollupServer+"/notice", "application/json", bytes.NewBuffer(data))
	return err
}

func main() {
	handlers := map[string]func(map[string]interface{}) (string, error){
		"advance_state": handleAdvance,
		"inspect_state": handleInspect,
	}

	for {
		resp, err := http.Post(rollupServer+"/finish", "application/json", bytes.NewBuffer([]byte(`{"status":"accept"}`)))
		if err != nil {
			log.Fatal(err)
		}

		if resp.StatusCode == http.StatusAccepted {
			log.Println("No pending rollup request, trying again")
			continue
		}

		var rollupReq map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &rollupReq)

		handlerType := rollupReq["request_type"].(string)
		handlerData := rollupReq["data"].(map[string]interface{})
		handler := handlers[handlerType]

		status, err := handler(handlerData)
		if err != nil {
			log.Println("Handler error:", err)
			continue
		}

		_, err = http.Post(rollupServer+"/finish", "application/json", bytes.NewBuffer([]byte(`{"status":"`+status+`"}`)))
		if err != nil {
			log.Fatal(err)
		}
	}
}
