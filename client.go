package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kjk/betterguid"
	"golang.org/x/net/context"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 2048
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	ctx context.Context
	// The cancel function for the context
	cancel  context.CancelFunc
	userUid string

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan ServerPush

	// Wait for reader and writer to close
	Wg sync.WaitGroup

	hub *Hub
}

func (c *Client) WaitForClose() {
	c.Wg.Wait()
}

func (c *Client) read(ctx context.Context, message chan<- ClientPush, done chan<- bool) {
	defer func() {
		done <- true
		close(message)
	}()

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("%s : error: %v", c.userUid, err)
			}
			log.Printf("%s : error: %v", c.userUid, err)
			return
		}
		msg = bytes.TrimSpace(bytes.Replace(msg, newline, space, -1))

		var clientPush ClientPush
		if err := json.Unmarshal(msg, &clientPush); err != nil {
			log.Fatalln("Could not convert message to struct")
		}

		select {
		case <-ctx.Done():
			return
		case message <- clientPush:
			// fmt.Println("received message", clientPush.Id)
		}

		// message <- clientPush

		// c.hub.broadcast <- message
	}
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	c.Wg.Add(1)
	defer func() {
		c.Wg.Done()
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	message := make(chan ClientPush, 1)
	done := make(chan bool, 1)
	go c.read(c.ctx, message, done)

	for {
		select {
		case <-c.ctx.Done():
			<-done
			log.Printf("%s : Closed ReadPump\n", c.userUid)
			return
		case clientPush, ok := <-message:

			if !ok {
				log.Printf("%s : Closed message channel\n", c.userUid)

				presence := AddPresenceModel{
					Id:        betterguid.New(),
					IsPresent: false,
					ChatId:    "",
					TaskId:    "",
					IsTyping:  false,
					SentBy:    c.userUid,
					Timestamp: time.Now().UTC(),
				}

				//save presence
				presenceMap[c.userUid] = presence

				//let my subscribers know that I went offline
				subscribers := presenceSubscribers[c.userUid]
				for _, s := range subscribers {
					m := ServerPush{
						Id:     presence.Id,
						UserId: s,
						Type:   ServerPushAddPresence,
						Data:   presence,
					}
					go hub.send(c.ctx, s, m, false)
				}

				//remove myself from the subscription lists
				for k, v := range presenceSubscribers {
					for i, u := range v {
						if u == c.userUid {
							v[i] = v[len(v)-1]
							presenceSubscribers[k] = v[:len(v)-1]
						}
					}
				}

				// c.cancel()
				return
			}

			// convert map to json
			data, _ := json.Marshal(clientPush.Data)
			// fmt.Println(string(data))

			ctx := context.WithValue(c.ctx, logPrefix, c.userUid+"/"+strconv.FormatUint(uint64(clientPush.Id), 10))
			if clientPush.Type == ClientPushAddGroup {
				// convert json to struct
				var chatGroup ChatGroup
				if err := json.Unmarshal(data, &chatGroup); err != nil {
					log.Fatalf("%s : Could not convert message data to struct", ctx.Value(logPrefix))
				}
				log.Printf("%s : Received AddGroup from %s = %s\n", ctx.Value(logPrefix), c.userUid, chatGroup.GroupId)
				handleAddGroup(ctx, chatGroup)
			}

			if clientPush.Type == ClientPushAddMessage {
				// convert json to struct
				var addMessageModel AddMessageModel
				if err := json.Unmarshal(data, &addMessageModel); err != nil {
					log.Fatalf("%s : Could not convert message data to struct", ctx.Value(logPrefix))
				}
				log.Printf("%s : Received AddMessage from %s = %s\n", ctx.Value(logPrefix), c.userUid, addMessageModel.Id)
				go handleTextMessage(ctx, addMessageModel, clientPush.Id)
			}

			if clientPush.Type == ClientPushAddThread {
				// convert json to struct
				var addThreadModel AddThreadModel
				if err := json.Unmarshal(data, &addThreadModel); err != nil {
					log.Fatalf("%s : Could not convert message data to struct", ctx.Value(logPrefix))
				}
				log.Printf("%s : Received AddThread from %s = %s\n", ctx.Value(logPrefix), c.userUid, addThreadModel.ThreadUid)
				handleAddThread(ctx, addThreadModel)
			}

			if clientPush.Type == ClientPushAck {
				// convert json to struct
				var messageId string
				if err := json.Unmarshal(data, &messageId); err != nil {
					log.Fatalf("%s : Could not convert message data to struct", ctx.Value(logPrefix))
				}

				// log.Printf("%s : Received PushAck from %s = %s\n", ctx.Value(logPrefix), c.userUid, messageId)
				go hadleClientAck(ctx, c.userUid, messageId)
			}

			if clientPush.Type == ClientPushPing {
				// log.Printf("%s : Received Ping from %s", ctx.Value(logPrefix), c.userUid)
				go hadleClientPing(ctx, clientPush.Id, c.userUid)
			}

			if clientPush.Type == ClientPushReadReceipt {
				// convert json to struct
				var receiptModel AddReadReceiptModel
				if err := json.Unmarshal(data, &receiptModel); err != nil {
					log.Fatalln("Could not convert message data to struct")
				}
				log.Printf("%s : Received ReadReceipt from %s = %v\n", ctx.Value(logPrefix), c.userUid, len(receiptModel.Receipts))
				handleReadReceipts(ctx, receiptModel, c.userUid, clientPush.Id)
			}

			if clientPush.Type == ClientPushAddTaskLogItem {
				// convert json to struct
				var update AddTaskLogItemModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert taskLogItem data to struct")
				}
				log.Printf("%s : Received TaskLogItem from %s\n", ctx.Value(logPrefix), c.userUid)
				handleTaskLogItem(ctx, update)
			}

			if clientPush.Type == ClientPushAddTask {
				// convert json to struct
				var update AddTaskModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert task data to struct")
				}
				log.Printf("%s : Received task from %s\n", ctx.Value(logPrefix), c.userUid)
				handleAddTask(ctx, update)
			}

			if clientPush.Type == ClientPushAddTaskStatus {
				// convert json to struct
				var update AddTaskStatusModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert task data to struct")
				}
				log.Printf("%s : Received task status from %s\n", ctx.Value(logPrefix), c.userUid)
				handleAddTaskStatus(ctx, update)
			}

			if clientPush.Type == ClientPushAddTaskMessage {
				// convert json to struct
				var update AddTaskMessageModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert task data to struct")
				}
				log.Printf("%s : Received task message from %s\n", ctx.Value(logPrefix), c.userUid)
				handleAddTaskMessage(ctx, update)
			}

			if clientPush.Type == ClientPushAddChat {
				// convert json to struct
				var update AddChatModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert task data to struct")
				}
				log.Printf("%s : Received chat from %s\n", ctx.Value(logPrefix), c.userUid)
				handleAddChat(ctx, update)
			}

			if clientPush.Type == ClientPushAddChatMessage {
				// convert json to struct
				var update AddChatMessageModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert task data to struct")
				}
				log.Printf("%s : Received chat message from %s\n", ctx.Value(logPrefix), c.userUid)
				handleAddChatMessage(ctx, update)
			}

			if clientPush.Type == ClientPushAddTaskReminder {
				// convert json to struct
				var update AddTaskReminderModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert task data to struct")
				}
				log.Printf("%s : Received task reminder from %s\n", ctx.Value(logPrefix), c.userUid)
				handleAddTaskReminder(ctx, update)
			}

			if clientPush.Type == ClientPushAddTaskDone {
				// convert json to struct
				var update AddTaskDoneModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert task data to struct")
				}
				log.Printf("%s : Received task done from %s\n", ctx.Value(logPrefix), c.userUid)
				handleAddTaskDone(ctx, update)
			}

			if clientPush.Type == ClientPushAddTaskNotDone {
				// convert json to struct
				var update AddTaskNotDoneModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert task data to struct")
				}
				log.Printf("%s : Received task not done from %s\n", ctx.Value(logPrefix), c.userUid)
				handleAddTaskNotDone(ctx, update)
			}

			if clientPush.Type == ClientPushAddWaitingRequest {
				// convert json to struct
				var update AddWaitingRequestModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert task data to struct")
				}
				log.Printf("%s : Received waiting request from %s\n", ctx.Value(logPrefix), c.userUid)
				handleAddWaitingRequest(ctx, update)
			}

			if clientPush.Type == ClientPushAcceptWaitingRequest {
				// convert json to struct
				var update AcceptWaitingRequestModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert waiting request accept data to struct")
				}
				log.Printf("%s : Received waiting request accept from %s\n", ctx.Value(logPrefix), c.userUid)
				handleAcceptWaitingRequest(ctx, update)
			}

			if clientPush.Type == ClientPushDenytWaitingRequest {
				// convert json to struct
				var update DenyWaitingRequestModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert waiting request deny data to struct")
				}
				log.Printf("%s : Received waiting request deny from %s\n", ctx.Value(logPrefix), c.userUid)
				handleDenyWaitingRequest(ctx, update)
			}

			if clientPush.Type == ClientPushAddChatGroup {
				// convert json to struct
				var update AddChatGroupModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert chat group data to struct")
				}
				log.Printf("%s : Received chat group from %s\n", ctx.Value(logPrefix), c.userUid)
				handleAddChatGroup(ctx, update)
			}

			if clientPush.Type == ClientPushAddChatGroupMember {
				// convert json to struct
				var update AddChatGroupMemberModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert chat group member data to struct")
				}
				log.Printf("%s : Received chat group member from %s\n", ctx.Value(logPrefix), c.userUid)
				handleAddChatGroupMember(ctx, update)
			}

			if clientPush.Type == ClientPushAddPresence {
				// convert json to struct
				var update AddPresenceModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert presence data to struct")
				}
				log.Printf("%s : Received presence from %s\n", ctx.Value(logPrefix), c.userUid)
				handleAddPresence(ctx, update)
			}

			if clientPush.Type == ClientPushAddGoodJobMessage {
				// convert json to struct
				var update AddGoodJobMessageModel
				if err := json.Unmarshal(data, &update); err != nil {
					log.Fatalln("Could not convert good job message data to struct")
				}
				log.Printf("%s : Received good job message from %s\n", ctx.Value(logPrefix), c.userUid)
				handleAddGoodJobMessage(ctx, update)
			}
		}
	}
}

func (c *Client) checkUndeliveredMessage() {
	messages, err := dbService.getMessages(c.ctx, c.userUid)

	if err != nil {
		log.Fatalln("Error fetching undelivered messages:", err)
	}

	for _, m := range messages {
		c.send <- m
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump(offset int64) {
	c.Wg.Add(1)
	ticker := time.NewTicker(pingPeriod)
	// done := make(chan bool, 1)
	defer func() {
		ticker.Stop()
		c.conn.Close()
		c.Wg.Done()
	}()

	go c.checkUndeliveredMessage()
	// go consume(c.ctx, c.userUid, offset, c.send, done)

	for {
		select {
		case <-c.ctx.Done():
			// wait for the consumer to close the reader
			log.Printf("%s : Closed WritePump\n", c.userUid)
			return
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			message, _ := json.Marshal(msg)

			// message := m.Value
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			// n := len(c.send)
			// for i := 0; i < n; i++ {
			// 	w.Write(newline)
			// 	w.Write(<-c.send)
			// }

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// serveWs handles websocket requests from the peer.
func serveWs(ctx context.Context, h *Hub, w http.ResponseWriter, r *http.Request, userUid string, offset int64) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	//TODO: is this okay? Gin context should not be used in a go routine.
	derivedCtx, cancel := context.WithCancel(ctx)
	client := &Client{
		ctx:     derivedCtx,
		cancel:  cancel,
		userUid: userUid,
		conn:    conn,
		send:    make(chan ServerPush, 256),
		hub:     h,
	}

	h.register <- client
	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump(offset)
	go client.readPump()

	presence := AddPresenceModel{
		Id:        betterguid.New(),
		IsPresent: true,
		ChatId:    "",
		TaskId:    "",
		IsTyping:  false,
		SentBy:    client.userUid,
		Timestamp: time.Now().UTC(),
	}
	presenceMap[client.userUid] = presence

	subscribers := presenceSubscribers[client.userUid]
	for _, s := range subscribers {
		m := ServerPush{
			Id:     presence.Id,
			UserId: s,
			Type:   ServerPushAddPresence,
			Data:   presence,
		}
		h.send(ctx, s, m, false)
	}
}
