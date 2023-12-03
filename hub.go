package main

import (
	"context"
	"log"
)

type Hub struct {
	clients    map[string]*Client
	register   chan *Client
	unregister chan *Client
	done       chan bool
}

func (h *Hub) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			for _, c := range h.clients {
				c.Wg.Wait()
			}
			h.done <- true
			return
		case c := <-h.register:
			h.clients[c.userUid] = c
		case c := <-h.unregister:
			delete(h.clients, c.userUid)
		}

	}
}

func (h *Hub) sendToChat(ctx context.Context, cid string, messageId string, contentType ServerPushType, data interface{}, waitForAck bool, sentBy string) {
	members, err := dbService.getChatGroupMembers(ctx, cid)
	if err != nil {
		log.Printf("Failed to fetch group members %v", err)
		return
	}

	for _, cgm := range members {
		if cgm.MemberUserId == sentBy {
			continue
		}

		pm := ServerPush{
			Id:     messageId,
			UserId: cgm.MemberUserId,
			Type:   contentType,
			Data:   data,
		}

		h.send(ctx, cgm.MemberUserId, pm, waitForAck)
	}
}

func (h *Hub) send(ctx context.Context, u string, m ServerPush, waitForAck bool) {
	// Don't log ping reply
	if waitForAck {
		log.Printf("%s : Pushing message to %s = %s\n", ctx.Value(logPrefix), u, serverPushTypeString(m.Type))
	}

	//save message to be delivered
	if waitForAck {
		// log.Println("Saving serverPush for user ", u)
		err := dbService.addMessage(ctx, m)
		if err != nil {
			log.Println("Saving serverPush for failed ", err.Error())
		}
	}

	//send message if client is connected
	//TODO Use mutex here. Can cause fatal error: concurrent map read and map write
	if c, exists := h.clients[u]; exists {
		// log.Println("Sending serverPush for user ", u)
		c.send <- m
	}
}

func serverPushTypeString(t ServerPushType) string {
	if t == ServerPushAddGroup {
		return "addGroup"
	}

	if t == ServerPushAddThread {
		return "addThread"
	}

	if t == ServerPushAddMessage {
		return "addMessage"
	}

	if t == ServerPushMessageReceipt {
		return "receipt"
	}

	if t == ServerPushChangeGroupName {
		return "changeGroupName"
	}

	if t == ServerPushAddGroupMember {
		return "addGroupMember"
	}

	if t == ServerPushRemoveGroupMember {
		return "removeGroupMember"
	}

	if t == ServerPushSystemMessage {
		return "systemMessage"
	}

	if t == ServerPushClientReply {
		return "reply"
	}

	if t == ServerPushAddChatId {
		return "addChatId"
	}

	if t == ServerPushAppContact {
		return "addAppContact"
	}

	if t == ServerPushUpdateThreadTitle {
		return "updateThreadTitle"
	}

	if t == ServerPushAddTask {
		return "task"
	}

	if t == ServerPushAddTaskStatus {
		return "taskStatus"
	}

	if t == ServerPushAddTaskMessage {
		return "taskMessage"
	}

	if t == ServerPushAddChat {
		return "chat"
	}

	if t == ServerPushAddChatMessage {
		return "chat message"
	}

	if t == ServerPushAddTaskReminder {
		return "task reminder"
	}

	if t == ServerPushAddTaskDone {
		return "task done"
	}

	if t == ServerPushAddTaskNotDone {
		return "task not done"
	}

	if t == ServerPushAddWaitingRequest {
		return "waiting request"
	}

	if t == ServerPushAcceptWaitingRequest {
		return "accept waiting request"
	}

	if t == ServerPushDenyWaitingRequest {
		return "deny waiting request"
	}

	if t == ServerPushAddChatGroup {
		return "chat group"
	}

	if t == ServerPushAddChatGroupMember {
		return "chat group member"
	}

	if t == ServerPushAddPresence {
		return "presence"
	}

	if t == ServerPushAddGoodJobMessage {
		return "good job"
	}

	return "unknown"
}
