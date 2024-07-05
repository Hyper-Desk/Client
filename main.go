package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/robfig/cron"
)

// VMInfo represents the information of a VM or CT
type VMInfo struct {
	UserID  string  `json:"userId"`
	Name    string  `json:"name"`
	VMID    int     `json:"vmid"`
	Type    string  `json:"type"`
	Status  string  `json:"status"`
	CPU     float64 `json:"cpu"`
	MaxCPU  int     `json:"maxcpu"`
	Mem     float64 `json:"mem"`     // Mem in GB
	MaxMem  float64 `json:"maxmem"`  // MaxMem in GB
	Disk    float64 `json:"disk"`    // Disk in TB
	MaxDisk float64 `json:"maxdisk"` // MaxDisk in TB
}

// LoginResponse represents the response structure for login
type LoginResponse struct {
	UserID       string `json:"userId"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

// LoginCredentials represents the user credentials for login
type LoginCredentials struct {
	ID       string `json:"userId"`
	Password string `json:"password"`
}

// Response represents the response structure
type Response struct {
	UserId string   `json:"userId"`
	Vms    []VMInfo `json:"vms"`
}

var userID string

func main() {
	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Replace with your server URL and API endpoints
	serverURL := os.Getenv("SERVER_URL")
	loginEndpoint := serverURL + "/api/user/login"
	vmListEndpoint := serverURL + "/api/vm/list"

	// Prompt user for credentials
	var credentials LoginCredentials
	fmt.Print("Enter your ID: ")
	fmt.Scanln(&credentials.ID)
	fmt.Print("Enter your password: ")
	fmt.Scanln(&credentials.Password)

	// Login and obtain token
	loginResp, err := login(credentials, loginEndpoint)
	if err != nil {
		log.Fatalf("Error logging in: %v", err)
	}

	userID = loginResp.UserID

	// Start cron job to send VM list every 5 minutes
	c := cron.New()
	c.AddFunc("*/5 * * * *", func() {
		vmList, err := getVMs()
		if err != nil {
			log.Printf("Error getting VM list: %v", err)
			return
		}

		response := Response{
			UserId: userID,
			Vms:    vmList.Vms,
		}

		err = sendToServer(response, vmListEndpoint)
		if err != nil {
			log.Printf("Error sending VM list to server: %v", err)
		}
	})
	c.Start()

	// Keep the program running
	select {}
}

// login sends a login request to the server and returns the access token
func login(credentials LoginCredentials, loginEndpoint string) (*LoginResponse, error) {
	// Convert credentials to JSON
	body, err := json.Marshal(credentials)
	if err != nil {
		return nil, err
	}

	// Send POST request to login endpoint
	resp, err := http.Post(loginEndpoint, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("login request failed: %v", err)
	}
	defer resp.Body.Close()

	// Check response status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("login failed with status code: %d", resp.StatusCode)
	}

	// Parse response body
	var loginResp LoginResponse
	err = json.NewDecoder(resp.Body).Decode(&loginResp)
	if err != nil {
		return nil, fmt.Errorf("failed to decode login response: %v", err)
	}

	return &loginResp, nil
}

// getVMs retrieves VM information from Proxmox VE
func getVMs() (*Response, error) {
	// Execute pvesh command to get VM list
	cmd := exec.Command("pvesh", "get", "/cluster/resources", "--output-format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute pvesh command: %v", err)
	}

	// Parse the JSON output
	var resources []VMInfo

	err = json.Unmarshal(output, &resources)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	// Convert to the desired structure
	vms := make([]VMInfo, 0)

	for _, res := range resources {
		vm := VMInfo{
			UserID:  userID,
			Name:    res.Name,
			VMID:    res.VMID,
			Type:    res.Type,
			Status:  res.Status,
			CPU:     res.CPU,
			MaxCPU:  res.MaxCPU,
			Mem:     float64(res.Mem) / (1024 * 1024),            // Convert from MB to GB
			MaxMem:  float64(res.MaxMem) / (1024 * 1024),         // Convert from MB to GB
			Disk:    float64(res.Disk) / (1024 * 1024 * 1024),    // Convert from GB to TB
			MaxDisk: float64(res.MaxDisk) / (1024 * 1024 * 1024), // Convert from GB to TB
		}

		// Round to two decimal places
		vm.Mem, _ = strconv.ParseFloat(fmt.Sprintf("%.2f", vm.Mem), 64)
		vm.MaxMem, _ = strconv.ParseFloat(fmt.Sprintf("%.2f", vm.MaxMem), 64)
		vm.Disk, _ = strconv.ParseFloat(fmt.Sprintf("%.2f", vm.Disk), 64)
		vm.MaxDisk, _ = strconv.ParseFloat(fmt.Sprintf("%.2f", vm.MaxDisk), 64)

		if res.Type == "qemu" || res.Type == "lxc" {
			vms = append(vms, vm)
		}
	}

	response := &Response{
		UserId: userID,
		Vms:    vms,
	}

	return response, nil
}

// sendToServer sends the VM list to the server
func sendToServer(vmList Response, serverURL string) error {
	data, err := json.Marshal(vmList)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", serverURL, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non-OK response: %s", resp.Status)
	}

	return nil
}
