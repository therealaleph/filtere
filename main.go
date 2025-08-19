package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
)

type CheckResult struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func buildCheckHostURL(ip, method string) (string, error) {
	switch method {
	case "http":
		return fmt.Sprintf("https://check-host.net/check-http?host=%s&node=ir1.node.check-host.net&node=ir2.node.check-host.net&node=ir3.node.check-host.net&node=ir5.node.check-host.net&node=ir6.node.check-host.net&node=ir7.node.check-host.net&node=ir8.node.check-host.net", ip), nil
	case "ping":
		return fmt.Sprintf("https://check-host.net/check-ping?host=%s&node=ir1.node.check-host.net&node=ir2.node.check-host.net&node=ir3.node.check-host.net&node=ir5.node.check-host.net&node=ir6.node.check-host.net&node=ir7.node.check-host.net&node=ir8.node.check-host.net", ip), nil
	case "dns":
		return fmt.Sprintf("https://check-host.net/check-dns?host=%s&node=ir1.node.check-host.net&node=ir2.node.check-host.net&node=ir3.node.check-host.net&node=ir5.node.check-host.net&node=ir6.node.check-host.net&node=ir7.node.check-host.net&node=ir8.node.check-host.net", ip), nil
	default:
		return "", fmt.Errorf("unsupported method: %s", method)
	}
}

func fetchCheckHostResult(requestID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("https://check-host.net/check-result/%s", requestID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func main() {
	app := fiber.New()

	app.Get("/check", func(c *fiber.Ctx) error {
		ip := c.Query("ip")
		method := c.Query("method")
		log.Printf("Received request: ip=%s, method=%s", ip, method)

		if ip == "" || method == "" {
			return c.Status(fiber.StatusBadRequest).JSON(CheckResult{
				Status:  "error",
				Message: "Missing 'ip' or 'method' query parameter.",
			})
		}

		url, err := buildCheckHostURL(ip, method)
		log.Printf("Calling check-host.net URL: %s", url)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(CheckResult{
				Status:  "error",
				Message: err.Error(),
			})
		}

		resultChan := make(chan CheckResult)

		go func() {
			client := &http.Client{Timeout: 10 * time.Second}
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				resultChan <- CheckResult{Status: "error", Message: err.Error()}
				return
			}
			req.Header.Set("Accept", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				resultChan <- CheckResult{Status: "error", Message: err.Error()}
				return
			}
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				resultChan <- CheckResult{Status: "error", Message: err.Error()}
				return
			}
			var apiResp map[string]interface{}
			if err := json.Unmarshal(body, &apiResp); err != nil {
				log.Printf("Non-JSON response from check-host.net: %s", string(body))
				resultChan <- CheckResult{Status: "error", Message: "check-host.net returned non-JSON response", Data: string(body)}
				return
			}
			requestID, ok := apiResp["request_id"].(string)
			if !ok {
				resultChan <- CheckResult{Status: "error", Message: "No request_id in response", Data: apiResp}
				return
			}
			var finalResult map[string]interface{}
			for i := 0; i < 60; i++ {
				res, err := fetchCheckHostResult(requestID)
				if err == nil && len(res) > 0 {
					anyNonNull := false
					for _, v := range res {
						if v != nil {
							anyNonNull = true
							break
						}
					}
					if anyNonNull {
						finalResult = res
						break
					}
				}
				time.Sleep(1 * time.Second)
			}
			if finalResult == nil {
				resultChan <- CheckResult{Status: "pending", Message: "Results not ready yet. Try again later.", Data: finalResult}
				return
			}
			resultChan <- CheckResult{Status: "ok", Data: finalResult}
		}()

		result := <-resultChan
		if result.Status == "error" {
			return c.Status(fiber.StatusInternalServerError).JSON(result)
		}
		return c.JSON(result)
	})

	log.Fatal(app.Listen(":3000"))
}
