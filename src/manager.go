package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var websocketUpgrader = websocket.Upgrader{
	CheckOrigin:     checkOrigin,
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Manager struct {
	clients ClientList
	sync.RWMutex
	handlers map[string]Eventhandler
	otps     RetentionMap
}

func NewManager(ctx context.Context) *Manager {
	m := &Manager{
		clients:  make(ClientList),
		handlers: make(map[string]Eventhandler),
		otps:     NewRetentionMap(ctx, 5*time.Second),
	}
	m.setupEventHandlers()
	return m

}

func (m *Manager) setupEventHandlers() {
	m.handlers[EventSendMessage] = SendMessage
}

func SendMessage(e Event, c *Client) error {
	fmt.Println("payload", e.Payload)
	return nil
}

func (m *Manager) routeEvent(event Event, c *Client) error {
	if handler, ok := m.handlers[event.Type]; ok {
		if err := handler(event, c); err != nil {
			return err
		}
		return nil
	} else {
		return errors.New("there is no such event type")
	}
}

func (m *Manager) serveWS(w http.ResponseWriter, r *http.Request) {
	otp := r.URL.Query().Get("otp")

	if otp == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if !m.otps.VerifyOTP(otp) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	log.Println("new connection")

	conn, err := websocketUpgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Println(err)
		return
	}

	Client := NewClient(conn, m)
	m.addClient(Client)

	go Client.readMessages()
	go Client.writeMessages()
}

func (m *Manager) addClient(client *Client) {
	// Lock so we can manipulate
	m.Lock()
	defer m.Unlock()

	// Add Client
	m.clients[client] = true
}

func (m *Manager) removeClient(client *Client) {
	m.Lock()
	defer m.Unlock()

	if _, ok := m.clients[client]; ok {
		client.connection.Close()
		delete(m.clients, client)
	}
}

func (m *Manager) loginHandler(w http.ResponseWriter, r *http.Request) {

	type userLoginRequest struct {
		Username string `json:"username"`
		Password string `json: "password"`
	}

	var req userLoginRequest

	err := json.NewDecoder(r.Body).Decode(&req)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Username == "percy" && req.Password == "123" {
		fmt.Printf("login success")
		type response struct {
			OTP string `json:"otp"`
		}

		otp := m.otps.NewOTP()

		resp := response{
			OTP: otp.Key,
		}

		data, err := json.Marshal(resp)

		if err != nil {
			log.Println(err)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(data)
		return

	}
	w.WriteHeader(http.StatusUnauthorized)
}

func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")

	switch origin {
	case "http://localhost:8080":
		return true
	default:
		return false
	}
}
