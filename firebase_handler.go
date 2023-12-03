package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/kjk/betterguid"
)

func handleAddGroup(ctx context.Context, chatGroup ChatGroup) {
	groupsRef := firebaseDbClient.NewRef(pathGroups)
	err := groupsRef.Child(chatGroup.GroupId).Set(ctx, chatGroup)
	if err != nil {
		log.Fatalln("Error setting value:", err)
	}

	// guid := betterguid.New()

	for _, member := range chatGroup.Members {
		if member.Uid == chatGroup.Author {
			continue
		}

		userChatGroup := chatGroup

		if userChatGroup.IsChat {
			partner, _ := findChatPartner(member, userChatGroup.Members)
			userChatGroup.GroupName = partner.PhoneNumber
		}

		serverPush := ServerPush{
			Id:   chatGroup.GroupId,
			Type: ServerPushAddGroup,
			Data: userChatGroup,
		}

		hub.send(ctx, member.Uid, serverPush, true)
	}

	log.Printf("%s : Sent addGroup message to %v members\n", ctx.Value(logPrefix), len(chatGroup.Members))

	if chatGroup.IsChat {
		childUpdates := make(map[string]interface{})

		for _, member := range chatGroup.Members {
			partner, _ := findChatPartner(member, chatGroup.Members)
			childUpdates["/"+member.Uid+"/"+partner.PhoneNumber] = chatGroup.GroupId
			chatId := ChatId{
				GroupId:     chatGroup.GroupId,
				PhoneNumber: partner.PhoneNumber,
			}
			serverPush := ServerPush{
				Id:   betterguid.New(),
				Type: ServerPushAddChatId,
				Data: chatId,
			}

			hub.send(ctx, member.Uid, serverPush, true)
		}

		userChatIdsRef := firebaseDbClient.NewRef(pathUserChatIds)

		if err := userChatIdsRef.Update(ctx, childUpdates); err != nil {
			log.Fatalln("Error updating children:", err)
		}
		log.Printf("%s : Updated Chat Ids\n", ctx.Value(logPrefix))
	}
}

type AddMessageModel struct {
	Id                 string    `json:"id"`
	Author             string    `json:"author"`
	Sender             string    `json:"sender"`
	Timestamp          time.Time `json:"timestamp"`
	Channel            string    `json:"channel,omitempty"`
	Group              string    `json:"group"`
	Message            Message   `json:"message"`
	Thread             string    `json:"thread,omitempty"`
	ReplyingInThreadTo string    `json:"replyingInThreadTo,omitempty"`
	SenderPublicKey    string    `json:"senderPublicKey"`
}

type MessageReceiptType int16

const (
	None MessageReceiptType = iota
	Sent
	Delivered
	Read
)

type MessageReceiptModel struct {
	Type      MessageReceiptType `json:"type" dynamodbav:"type"`
	MessageId string             `json:"messageId" dynamodbav:"messageId"`
	AppUser   string             `json:"appUser" dynamodbav:"appUser"`
	Timestamp time.Time          `json:"timestamp" dynamodbav:"timestamp"`
}

type MessageData struct {
	SenderDisplayName string
	// Thread or Channel Id
	ThreadId string
	// Thread name or Channel name
	ThreadName string
	Recepients []AppUser
}

func getRecepientsForMessage(ctx context.Context, message AddMessageModel) (MessageData, error) {
	var messageData MessageData
	var members []AppUser

	if len(message.Channel) != 0 {
		groupsRef := firebaseDbClient.NewRef(pathGroups)

		var chatGroup ChatGroup
		if err := groupsRef.Child(message.Group).Get(ctx, &chatGroup); err != nil {
			log.Fatalln("Error reading value for chatGroup: "+message.Group, err)
		}

		for _, member := range chatGroup.Members {
			if member.Uid == message.Author {
				continue
			}
			members = append(members, member)
		}
		messageData = MessageData{
			ThreadId:   message.Channel,
			ThreadName: chatGroup.GroupName,
			Recepients: members,
		}
	} else {
		threadsRef := firebaseDbClient.NewRef(pathThreads)

		var chatThread ChatThread
		if err := threadsRef.Child(message.Thread).Get(ctx, &chatThread); err != nil {
			log.Fatalln("Error reading value for ChatThread: "+message.Group, err)
		}

		for _, member := range chatThread.Members {
			if member.Uid == message.Author {
				continue
			}
			members = append(members, member)
		}

		messageData = MessageData{
			ThreadId:   message.Thread,
			ThreadName: chatThread.Title.Content,
			Recepients: members,
		}
	}

	return messageData, nil
}

func handleTextMessage(ctx context.Context, message AddMessageModel, replyId uint32) {
	// send receipt
	messageReceipt := MessageReceiptModel{
		Type:      Sent,
		MessageId: message.Id,
	}

	// receiptId := betterguid.New()

	// serverPushReceipt := ServerPush{
	// 	Id:   receiptId,
	// 	Type: ServerPushMessageReceipt,
	// 	Data: messageReceipt,
	// }

	// go hub.send(ctx, message.Author, serverPushReceipt, true)

	reply := ServerPush{
		Id:   betterguid.New(),
		Type: ServerPushClientReply,
		Data: ClientReplyModel{
			Id:     replyId,
			Result: messageReceipt,
		},
	}

	hub.send(ctx, message.Author, reply, false)

	//handle message

	messageData, err := getRecepientsForMessage(ctx, message)
	if err != nil {
		log.Fatalln("Error loading members: "+message.Group, err)
	}

	for _, member := range messageData.Recepients {
		serverPush := ServerPush{
			Id:   message.Id,
			Type: ServerPushAddMessage,
			Data: message,
		}

		hub.send(ctx, member.Uid, serverPush, true)
	}

	// log.Printf("%s : Sent message to %v members\n", ctx.Value(logPrefix), len(chatGroup.Members))
	go sendNewMessageNotification(ctx, message, messageData.ThreadId, messageData.ThreadName, messageData.Recepients)
}

type AddThreadModel struct {
	Author     string             `json:"author"`
	ThreadUid  string             `json:"threadUid"`
	Group      string             `json:"group"`
	Title      Message            `json:"title"`
	ReplyingTo string             `json:"replyingTo,omitempty"`
	Members    map[string]AppUser `json:"members"`
}

type SystemMessageType int16

const (
	SystemMessageAddThread         SystemMessageType = 1
	SystemMessageUpdateThreadTitle SystemMessageType = 6
)

type AddSystemMessageModel struct {
	Type      SystemMessageType `json:"type"`
	Id        string            `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Channel   string            `json:"channel,omitempty"`
	Group     string            `json:"group"`
	Thread    string            `json:"thread,omitempty"`
	Message   Message           `json:"message"`
	Context   interface{}       `json:"context,omitempty"`
}

type SystemMessageAddThreadContext struct {
	ThreadUid   string  `json:"threadUid"`
	ThreadTitle Message `json:"threadTitle"`
}

func handleAddThread(ctx context.Context, addThreadModel AddThreadModel) {
	threadsRef := firebaseDbClient.NewRef(pathThreads)
	err := threadsRef.Child(addThreadModel.ThreadUid).Set(ctx, addThreadModel)
	if err != nil {
		log.Fatalln("Error setting value:", err)
	}

	var chatGroup ChatGroup
	groupsRef := firebaseDbClient.NewRef(pathGroups)
	if err := groupsRef.Child(addThreadModel.Group).Get(ctx, &chatGroup); err != nil {
		log.Fatalln("Error reading value:", err)
	}

	var currAppUser AppUser
	usersRef := firebaseDbClient.NewRef(pathUsers)
	if err := usersRef.Child(addThreadModel.Author).Get(ctx, &currAppUser); err != nil {
		log.Fatalln("Error reading value:", err)
	}

	// var recepients []AppUser

	for _, member := range addThreadModel.Members {

		if member.Uid == addThreadModel.Author {
			continue
		}

		// recepients = append(recepients, member)

		serverPush := ServerPush{
			Id:   addThreadModel.ThreadUid,
			Type: ServerPushAddThread,
			Data: addThreadModel,
		}

		hub.send(ctx, member.Uid, serverPush, true)
	}

	// systemMessage := AddSystemMessageModel{
	// 	Type:      SystemMessageAddThread,
	// 	Id:        betterguid.New(),
	// 	Timestamp: time.Now().UTC(),
	// 	Channel:   chatGroup.DefaultChannel.ChannelId,
	// 	Group:     addThreadModel.Group,
	// 	Message: Message{
	// 		Content: "@" + currAppUser.PhoneNumber + " added a new thread",
	// 		Mentions: []Mention{{
	// 			Id:          betterguid.New(),
	// 			Uid:         addThreadModel.Author,
	// 			PhoneNumber: currAppUser.PhoneNumber,
	// 			Range:       [2]int{0, len(currAppUser.PhoneNumber) + 1},
	// 		}},
	// 		Links: make([]string, 0),
	// 	},
	// 	Context: SystemMessageAddThreadContext{
	// 		ThreadUid:   addThreadModel.ThreadUid,
	// 		ThreadTitle: addThreadModel.Title,
	// 	},
	// }

	// for _, member := range addThreadModel.Members {
	// 	serverPushSystemMessage := ServerPush{
	// 		Id:   systemMessage.Id,
	// 		Type: ServerPushSystemMessage,
	// 		Data: systemMessage,
	// 	}

	// 	hub.send(ctx, member.Uid, serverPushSystemMessage, true)
	// }

	// go sendNewThreadNotification(ctx, currAppUser.PhoneNumber, addThreadModel.ThreadUid, addThreadModel.Title.Content, recepients)
}

type AddAckModel struct {
	MessageId string `json:"messageId"`
}

func hadleClientAck(c context.Context, uid string, mid string) {
	pm, err := dbService.getMessageById(c, mid, uid)
	if err != nil {
		log.Fatalln("Failed to fetch message:", err)
	}

	//delete message after delivery is confirmed
	if err := dbService.removeMessageById(c, mid, uid); err != nil {
		log.Fatalln("Error removing delivered message:", err)
	}

	//send delivered receipt to message author
	if pm.Type == ServerPushAddChatMessage {
		jsonString, _ := json.Marshal(pm.Data)
		var m AddChatMessageModel
		err := json.Unmarshal(jsonString, &m)
		if err != nil {
			log.Println("Could not convert data to chat message")
		}

		sendDeliveryReceipt(c, uid, mid, m.SentBy, true)
	} else if pm.Type == ServerPushAddTaskStatus {
		jsonString, _ := json.Marshal(pm.Data)
		var m AddTaskStatusModel
		err := json.Unmarshal(jsonString, &m)
		if err != nil {
			log.Println("Could not convert data to task status message")
		}

		sendDeliveryReceipt(c, uid, mid, m.SentBy, true)
	} else if pm.Type == ServerPushAddTaskMessage {
		jsonString, _ := json.Marshal(pm.Data)
		var m AddTaskMessageModel
		err := json.Unmarshal(jsonString, &m)
		if err != nil {
			log.Println("Could not convert data to task text message")
		}

		sendDeliveryReceipt(c, uid, mid, m.SentBy, true)
	} else if pm.Type == ServerPushAddTaskReminder {
		jsonString, _ := json.Marshal(pm.Data)
		var m AddTaskReminderModel
		err := json.Unmarshal(jsonString, &m)
		if err != nil {
			log.Println("Could not convert data to task reminder message")
		}

		sendDeliveryReceipt(c, uid, mid, m.SentBy, true)
	} else if pm.Type == ServerPushAddTaskDone {
		jsonString, _ := json.Marshal(pm.Data)
		var m AddTaskDoneModel
		err := json.Unmarshal(jsonString, &m)
		if err != nil {
			log.Println("Could not convert data to task reminder message")
		}

		sendDeliveryReceipt(c, uid, mid, m.SentBy, true)
	} else if pm.Type == ServerPushAddTaskNotDone {
		jsonString, _ := json.Marshal(pm.Data)
		var m AddTaskNotDoneModel
		err := json.Unmarshal(jsonString, &m)
		if err != nil {
			log.Println("Could not convert data to task reminder message")
		}

		sendDeliveryReceipt(c, uid, mid, m.SentBy, true)
	} else if pm.Type == ServerPushAddWaitingRequest {
		jsonString, _ := json.Marshal(pm.Data)
		var m AddWaitingRequestModel
		err := json.Unmarshal(jsonString, &m)
		if err != nil {
			log.Println("Could not convert data to task reminder message")
		}

		sendDeliveryReceipt(c, uid, mid, m.SentBy, true)
	} else if pm.Type == ServerPushAcceptWaitingRequest {
		jsonString, _ := json.Marshal(pm.Data)
		var m AcceptWaitingRequestModel
		err := json.Unmarshal(jsonString, &m)
		if err != nil {
			log.Println("Could not convert data to task accept waiting request message")
		}

		sendDeliveryReceipt(c, uid, mid, m.SentBy, true)
	} else if pm.Type == ServerPushDenyWaitingRequest {
		jsonString, _ := json.Marshal(pm.Data)
		var m DenyWaitingRequestModel
		err := json.Unmarshal(jsonString, &m)
		if err != nil {
			log.Println("Could not convert data to task deny waiting request message")
		}

		sendDeliveryReceipt(c, uid, mid, m.SentBy, true)
	}
}

func sendDeliveryReceipt(ctx context.Context, uid string, mid string, rid string, waitForAck bool) {
	dr := MessageReceiptModel{
		Type:      Delivered,
		MessageId: mid,
		AppUser:   uid,
		Timestamp: time.Now().UTC(),
	}

	receiptId := betterguid.New()

	sp := ServerPush{
		Id:     receiptId,
		UserId: rid,
		Type:   ServerPushMessageReceipt,
		Data:   dr,
	}

	go hub.send(ctx, rid, sp, true)
}

type ServerErrorModel struct {
	Code    uint32 `json:"code"`
	Message string `json:"message"`
}

type ClientReplyModel struct {
	Id     uint32            `json:"id"`
	Error  *ServerErrorModel `json:"error,omitempty"`
	Result interface{}       `json:"result,omitempty"`
}

func hadleClientPing(ctx context.Context, replyId uint32, userID string) {
	serverPushReceipt := ServerPush{
		Id:   betterguid.New(),
		Type: ServerPushClientReply,
		Data: ClientReplyModel{
			Id: replyId,
		},
	}

	hub.send(ctx, userID, serverPushReceipt, false)
}

func handleReadReceipts(ctx context.Context, receipts AddReadReceiptModel, uid string, replyId uint32) {
	for _, receipt := range receipts.Receipts {
		messageReceipt := MessageReceiptModel{
			Type:      Read,
			MessageId: receipt.MessageId,
			AppUser:   uid,
			Timestamp: time.Now().UTC(),
		}

		receiptId := betterguid.New()

		serverPushReceipt := ServerPush{
			Id:     receiptId,
			UserId: receipt.Author,
			Type:   ServerPushMessageReceipt,
			Data:   messageReceipt,
		}

		go hub.send(ctx, receipt.Author, serverPushReceipt, true)
	}

	serverPushReply := ServerPush{
		Id:     betterguid.New(),
		UserId: uid,
		Type:   ServerPushClientReply,
		Data: ClientReplyModel{
			Id: replyId,
		},
	}

	hub.send(ctx, uid, serverPushReply, false)
}

type AddTaskModel struct {
	Id                           string    `json:"id" dynamodbav:"id"`
	Title                        string    `json:"title" dynamodbav:"title"`
	Description                  string    `json:"description" dynamodbav:"description"`
	AssginedTo                   string    `json:"assignedTo" dynamodbav:"assignedTo"`
	AssignedBy                   string    `json:"assignedBy" dynamodbav:"assignedBy"`
	IsUrgent                     bool      `json:"isUrgent" dynamodbav:"isUrgent"`
	DueDate                      time.Time `json:"dueDate" dynamodbav:"dueDate"`
	GroupUid                     string    `json:"groupUid" dynamodbav:"groupUid"`
	IsRepeatWeekly               bool      `json:"isRepeatWeekly" dynamodbav:"isRepeatWeekly"`
	RepeatWeekday                int8      `json:"repeatWeekday" dynamodbav:"repeatWeekday"`
	IsRepeatingTask              bool      `json:"isRepeatingTask" dynamodbav:"isRepeatingTask"`
	TaskRepeatType               int8      `json:"taskRepeatType" dynamodbav:"taskRepeatType"`
	LatestRecurringTaskCreatedAt time.Time `json:"latestRecurringTaskCreatedAt" dynamodbav:"latestRecurringTaskCreatedAt"`
}

type AddTaskLogItemModel struct {
	Id             string       `json:"id"`
	Task           AddTaskModel `json:"task"`
	CreatedBy      string       `json:"createdBy"`
	Message        Message      `json:"description"`
	Status         TaskStatus   `json:"status"`
	Timestamp      time.Time    `json:"timestamp"`
	PendingDueDate time.Time    `json:"pendingDueDate"`
}

type TaskStatus int16

const (
	TaskStatusOpen TaskStatus = iota
	TaskStatusWaiting
	TaskStatusCompleted
)

func handleTaskLogItem(ctx context.Context, update AddTaskLogItemModel) {
	// send receipt
	messageReceipt := MessageReceiptModel{
		Type:      Sent,
		MessageId: update.Id,
	}

	receiptId := betterguid.New()

	serverPushReceipt := ServerPush{
		Id:   receiptId,
		Type: ServerPushMessageReceipt,
		Data: messageReceipt,
	}

	go hub.send(ctx, update.CreatedBy, serverPushReceipt, true)

	//Retrive chat group.

	groupsRef := firebaseDbClient.NewRef(pathGroups)

	var chatGroup ChatGroup
	if err := groupsRef.Child(update.Task.GroupUid).Get(ctx, &chatGroup); err != nil {
		log.Fatalln("Error reading value for chatGroup: "+update.Task.GroupUid, err)
	}

	var recepients []AppUser

	for _, member := range chatGroup.Members {
		if member.Uid == update.CreatedBy {
			continue
		}
		recepients = append(recepients, member)
	}

	//Create automated message with status change mentioning task and optional message and due date (for pending).

	var currAppUser AppUser

	usersRef := firebaseDbClient.NewRef(pathUsers)
	if err := usersRef.Child(update.CreatedBy).Get(ctx, &currAppUser); err != nil {
		log.Fatalln("Error reading value:", err)
	}

	//Send task log entry to all members in the group.
	for _, member := range recepients {
		serverPushTaskLogItem := ServerPush{
			Id:   update.Id,
			Type: ServerPushAddTaskLogItem,
			Data: update,
		}

		hub.send(ctx, member.Uid, serverPushTaskLogItem, true)
	}

	if update.Status == TaskStatusOpen {
		//Send new task notification to assignee.
		var assignee AppUser
		if err := usersRef.Child(update.Task.AssginedTo).Get(ctx, &assignee); err != nil {
			log.Fatalln("Error reading value:", err)
		}

		go sendNewTaskNotification(ctx, update.Task, []AppUser{assignee})

	} else if update.Status == TaskStatusCompleted {
		//Send new task notification to assignee.
		go sendTaskCompletedNotification(ctx, update.Task, recepients)
	} else if update.Status == TaskStatusWaiting {
		//Send task log entry to all members in the group.
		for _, member := range recepients {
			serverPushTaskLogItem := ServerPush{
				Id:   update.Id,
				Type: ServerPushAddTaskLogItem,
				Data: update,
			}

			hub.send(ctx, member.Uid, serverPushTaskLogItem, true)
		}

		//Send new task notification to assignee.
		go sendTaskPendingNotification(ctx, update.Task, recepients)
	}
}

func handleAddTask(ctx context.Context, task AddTaskModel) {
	//save task to db
	dbService.addTask(ctx, task)

	//send task
	go hub.sendToChat(ctx, task.GroupUid, task.Id, ServerPushAddTask, task, true, task.AssignedBy)

	go notificationService.sendNewTaskNotification(ctx, task, task.Id)
}

type AddTaskStatusModel struct {
	Id        string     `json:"id" dynamodbav:"id"`
	TaskId    string     `json:"taskId" dynamodbav:"taskId"`
	ChatId    string     `json:"chatId" dynamodbav:"chatId"`
	Status    TaskStatus `json:"status" dynamodbav:"status"`
	SentTo    string     `json:"sentTo" dynamodbav:"sentTo"`
	SentBy    string     `json:"sentBy" dynamodbav:"sentBy"`
	DueDate   time.Time  `json:"dueDate" dynamodbav:"dueDate"`
	Timestamp time.Time  `json:"timestamp" dynamodbav:"timestamp"`
}

func handleAddTaskStatus(ctx context.Context, update AddTaskStatusModel) {
	// send receipt
	messageReceipt := MessageReceiptModel{
		Type:      Sent,
		MessageId: update.Id,
		Timestamp: time.Now().UTC(),
	}

	receiptId := betterguid.New()

	serverPushReceipt := ServerPush{
		Id:     receiptId,
		UserId: update.SentBy,
		Type:   ServerPushMessageReceipt,
		Data:   messageReceipt,
	}

	go hub.send(ctx, update.SentBy, serverPushReceipt, true)

	//send status
	go hub.sendToChat(ctx, update.ChatId, update.Id, ServerPushAddTaskStatus, update, true, update.SentBy)
}

type AddTaskMessageModel struct {
	Id        string    `json:"id" dynamodbav:"id"`
	TaskId    string    `json:"taskId" dynamodbav:"taskId"`
	ChatId    string    `json:"chatId" dynamodbav:"chatId"`
	Message   string    `json:"message" dynamodbav:"message"`
	SentTo    string    `json:"sentTo" dynamodbav:"sentTo"`
	SentBy    string    `json:"sentBy" dynamodbav:"sentBy"`
	TaskTitle string    `json:"taskTitle" dynamodbav:"taskTitle"`
	Timestamp time.Time `json:"timestamp" dynamodbav:"timestamp"`
}

func handleAddTaskMessage(ctx context.Context, m AddTaskMessageModel) {
	// send receipt
	mr := MessageReceiptModel{
		Type:      Sent,
		MessageId: m.Id,
		Timestamp: time.Now().UTC(),
	}

	receiptId := betterguid.New()

	pr := ServerPush{
		Id:     receiptId,
		UserId: m.SentBy,
		Type:   ServerPushMessageReceipt,
		Data:   mr,
	}

	go hub.send(ctx, m.SentBy, pr, true)

	//send task text message
	go hub.sendToChat(ctx, m.ChatId, m.Id, ServerPushAddTaskMessage, m, true, m.SentBy)

	// send notification
	notificationService.sendTaskTextMessageNotification(ctx, m.Message, m.SentBy, m.SentTo, m.TaskId, m.TaskTitle, m.Id)
}

type AddChatModel struct {
	Id        string    `json:"id" dynamodbav:"id"`
	SentTo    string    `json:"sentTo" dynamodbav:"sentTo"`
	SentBy    string    `json:"sentBy" dynamodbav:"sentBy"`
	CreatedAt time.Time `json:"createdAt" dynamodbav:"createdAt"`
}

func handleAddChat(ctx context.Context, c AddChatModel) {
	//send new chat to partner
	pm := ServerPush{
		Id:     betterguid.New(),
		UserId: c.SentTo,
		Type:   ServerPushAddChat,
		Data:   c,
	}

	//save one to one chat as group chat
	cg := AddChatGroupModel{
		Id:        c.Id,
		Title:     "onetoone",
		SentBy:    c.SentBy,
		CreatedAt: c.CreatedAt,
	}

	dbService.addChatGroup(ctx, cg)

	//save sender as chat group member
	ogm := AddChatGroupMemberModel{
		Id:           betterguid.New(),
		ChatId:       c.Id,
		MemberUserId: c.SentBy,
		SentBy:       c.SentBy,
	}

	dbService.addChatGroupMember(ctx, ogm)

	//save chat partner as chat group member
	pgm := AddChatGroupMemberModel{
		Id:           betterguid.New(),
		ChatId:       c.Id,
		MemberUserId: c.SentTo,
		SentBy:       c.SentBy,
	}

	dbService.addChatGroupMember(ctx, pgm)

	//send chat to partner
	hub.send(ctx, c.SentTo, pm, true)
}

type AddChatMessageModel struct {
	Id        string    `json:"id" dynamodbav:"id"`
	ChatId    string    `json:"chatId" dynamodbav:"chatId"`
	Message   string    `json:"message" dynamodbav:"message"`
	SentTo    string    `json:"sentTo" dynamodbav:"sentTo"`
	SentBy    string    `json:"sentBy" dynamodbav:"sentBy"`
	Timestamp time.Time `json:"timestamp" dynamodbav:"timestamp"`
}

func handleAddChatMessage(ctx context.Context, m AddChatMessageModel) {
	// send receipt
	mr := MessageReceiptModel{
		Type:      Sent,
		MessageId: m.Id,
		Timestamp: time.Now().UTC(),
	}

	receiptId := betterguid.New()

	pr := ServerPush{
		Id:     receiptId,
		UserId: m.SentBy,
		Type:   ServerPushMessageReceipt,
		Data:   mr,
	}

	go hub.send(ctx, m.SentBy, pr, true)

	//send new chat message to assignee
	go hub.sendToChat(ctx, m.ChatId, m.Id, ServerPushAddChatMessage, m, true, m.SentBy)

	notificationService.sendChatTextMessageNotification(ctx, m.Message, m.SentBy, m.ChatId, m.Id)
}

type AddTaskReminderModel struct {
	Id        string    `json:"id" dynamodbav:"id"`
	TaskId    string    `json:"taskId" dynamodbav:"taskId"`
	TaskTitle string    `json:"taskTitle" dynamodbav:"taskTitle"`
	ChatId    string    `json:"chatId" dynamodbav:"chatId"`
	SentTo    string    `json:"sentTo" dynamodbav:"sentTo"`
	SentBy    string    `json:"sentBy" dynamodbav:"sentBy"`
	Timestamp time.Time `json:"timestamp" dynamodbav:"timestamp"`
}

func handleAddTaskReminder(ctx context.Context, m AddTaskReminderModel) {
	// send receipt
	mr := MessageReceiptModel{
		Type:      Sent,
		MessageId: m.Id,
		Timestamp: time.Now().UTC(),
	}

	receiptId := betterguid.New()

	pr := ServerPush{
		Id:     receiptId,
		UserId: m.SentBy,
		Type:   ServerPushMessageReceipt,
		Data:   mr,
	}

	// send receipt
	go hub.send(ctx, m.SentBy, pr, true)

	//send reminder
	go hub.sendToChat(ctx, m.ChatId, m.Id, ServerPushAddTaskReminder, m, true, m.SentBy)

	// send notification
	notificationService.sendTaskReminderNotification(ctx, m.SentBy, m.SentTo, m.TaskId, m.TaskTitle, m.Id)
}

type AddTaskDoneModel struct {
	Id        string    `json:"id" dynamodbav:"id"`
	TaskId    string    `json:"taskId" dynamodbav:"taskId"`
	ChatId    string    `json:"chatId" dynamodbav:"chatId"`
	TaskTitle string    `json:"taskTitle" dynamodbav:"taskTitle"`
	SentTo    string    `json:"sentTo" dynamodbav:"sentTo"`
	SentBy    string    `json:"sentBy" dynamodbav:"sentBy"`
	DueDate   time.Time `json:"dueDate" dynamodbav:"dueDate"`
	Timestamp time.Time `json:"timestamp" dynamodbav:"timestamp"`
}

func handleAddTaskDone(ctx context.Context, m AddTaskDoneModel) {
	// send receipt
	mr := MessageReceiptModel{
		Type:      Sent,
		MessageId: m.Id,
		Timestamp: time.Now().UTC(),
	}

	receiptId := betterguid.New()

	pr := ServerPush{
		Id:     receiptId,
		UserId: m.SentBy,
		Type:   ServerPushMessageReceipt,
		Data:   mr,
	}

	go hub.send(ctx, m.SentBy, pr, true)

	//send done
	go hub.sendToChat(ctx, m.ChatId, m.Id, ServerPushAddTaskDone, m, true, m.SentBy)

	//send notification
	notificationService.sendTaskDoneNotification(ctx, m.SentBy, m.SentTo, m.TaskId, m.TaskTitle, m.Id)
}

type AddTaskNotDoneModel struct {
	Id        string    `json:"id" dynamodbav:"id"`
	TaskId    string    `json:"taskId" dynamodbav:"taskId"`
	ChatId    string    `json:"chatId" dynamodbav:"chatId"`
	TaskTitle string    `json:"taskTitle" dynamodbav:"taskTitle"`
	SentTo    string    `json:"sentTo" dynamodbav:"sentTo"`
	SentBy    string    `json:"sentBy" dynamodbav:"sentBy"`
	DueDate   time.Time `json:"dueDate" dynamodbav:"dueDate"`
	Timestamp time.Time `json:"timestamp" dynamodbav:"timestamp"`
}

func handleAddTaskNotDone(ctx context.Context, m AddTaskNotDoneModel) {
	// send receipt
	mr := MessageReceiptModel{
		Type:      Sent,
		MessageId: m.Id,
		Timestamp: time.Now().UTC(),
	}

	receiptId := betterguid.New()

	pr := ServerPush{
		Id:     receiptId,
		UserId: m.SentBy,
		Type:   ServerPushMessageReceipt,
		Data:   mr,
	}

	go hub.send(ctx, m.SentBy, pr, true)

	//send not done
	go hub.sendToChat(ctx, m.ChatId, m.Id, ServerPushAddTaskNotDone, m, true, m.SentBy)

	notificationService.sendTaskNotDoneNotification(ctx, m.SentBy, m.SentTo, m.TaskId, m.TaskTitle, m.Id)
}

type AddWaitingRequestModel struct {
	Id        string    `json:"id" dynamodbav:"id"`
	TaskId    string    `json:"taskId" dynamodbav:"taskId"`
	ChatId    string    `json:"chatId" dynamodbav:"chatId"`
	TaskTitle string    `json:"taskTitle" dynamodbav:"taskTitle"`
	SentTo    string    `json:"sentTo" dynamodbav:"sentTo"`
	SentBy    string    `json:"sentBy" dynamodbav:"sentBy"`
	DueDate   time.Time `json:"dueDate" dynamodbav:"dueDate"`
	Timestamp time.Time `json:"timestamp" dynamodbav:"timestamp"`
}

func handleAddWaitingRequest(ctx context.Context, m AddWaitingRequestModel) {
	// send receipt
	mr := MessageReceiptModel{
		Type:      Sent,
		MessageId: m.Id,
		Timestamp: time.Now().UTC(),
	}

	receiptId := betterguid.New()

	pr := ServerPush{
		Id:     receiptId,
		UserId: m.SentBy,
		Type:   ServerPushMessageReceipt,
		Data:   mr,
	}

	go hub.send(ctx, m.SentBy, pr, true)

	//send waiting request
	go hub.sendToChat(ctx, m.ChatId, m.Id, ServerPushAddWaitingRequest, m, true, m.SentBy)

	notificationService.sendTaskWaitingRequestNotification(ctx, m.SentBy, m.SentTo, m.TaskId, m.TaskTitle, m.Id)
}

type AcceptWaitingRequestModel struct {
	Id        string    `json:"id" dynamodbav:"id"`
	TaskId    string    `json:"taskId" dynamodbav:"taskId"`
	ChatId    string    `json:"chatId" dynamodbav:"chatId"`
	TaskTitle string    `json:"taskTitle" dynamodbav:"taskTitle"`
	SentTo    string    `json:"sentTo" dynamodbav:"sentTo"`
	SentBy    string    `json:"sentBy" dynamodbav:"sentBy"`
	RequestId string    `json:"requestId" dynamodbav:"requestId"`
	DueDate   time.Time `json:"dueDate" dynamodbav:"dueDate"`
	Timestamp time.Time `json:"timestamp" dynamodbav:"timestamp"`
}

func handleAcceptWaitingRequest(ctx context.Context, m AcceptWaitingRequestModel) {
	// send receipt
	mr := MessageReceiptModel{
		Type:      Sent,
		MessageId: m.Id,
		Timestamp: time.Now().UTC(),
	}

	receiptId := betterguid.New()

	pr := ServerPush{
		Id:     receiptId,
		UserId: m.SentBy,
		Type:   ServerPushMessageReceipt,
		Data:   mr,
	}

	go hub.send(ctx, m.SentBy, pr, true)

	//send accept
	go hub.sendToChat(ctx, m.ChatId, m.Id, ServerPushAcceptWaitingRequest, m, true, m.SentBy)

	notificationService.sendTaskAcceptWaitingRequestNotification(ctx, m.SentBy, m.SentTo, m.TaskId, m.TaskTitle, m.Id)
}

type DenyWaitingRequestModel struct {
	Id        string    `json:"id" dynamodbav:"id"`
	TaskId    string    `json:"taskId" dynamodbav:"taskId"`
	ChatId    string    `json:"chatId" dynamodbav:"chatId"`
	TaskTitle string    `json:"taskTitle" dynamodbav:"taskTitle"`
	SentTo    string    `json:"sentTo" dynamodbav:"sentTo"`
	SentBy    string    `json:"sentBy" dynamodbav:"sentBy"`
	RequestId string    `json:"requestId" dynamodbav:"requestId"`
	DueDate   time.Time `json:"dueDate" dynamodbav:"dueDate"`
	Timestamp time.Time `json:"timestamp" dynamodbav:"timestamp"`
}

func handleDenyWaitingRequest(ctx context.Context, m DenyWaitingRequestModel) {
	// send receipt
	mr := MessageReceiptModel{
		Type:      Sent,
		MessageId: m.Id,
		Timestamp: time.Now().UTC(),
	}

	receiptId := betterguid.New()

	pr := ServerPush{
		Id:     receiptId,
		UserId: m.SentBy,
		Type:   ServerPushMessageReceipt,
		Data:   mr,
	}

	go hub.send(ctx, m.SentBy, pr, true)

	//send deny
	go hub.sendToChat(ctx, m.ChatId, m.Id, ServerPushDenyWaitingRequest, m, true, m.SentBy)

	notificationService.sendTaskDenyWaitingRequestNotification(ctx, m.SentBy, m.SentTo, m.TaskId, m.TaskTitle, m.Id)
}

type AddChatGroupModel struct {
	Id        string    `json:"id" dynamodbav:"id"`
	Title     string    `json:"title" dynamodbav:"title"`
	SentBy    string    `json:"sentBy" dynamodbav:"sentBy"`
	CreatedAt time.Time `json:"createdAt" dynamodbav:"createdAt"`
}

func handleAddChatGroup(ctx context.Context, m AddChatGroupModel) {
	//save group
	dbService.addChatGroup(ctx, m)

	//add group member
	cgm := AddChatGroupMemberModel{
		Id:           betterguid.New(),
		ChatId:       m.Id,
		MemberUserId: m.SentBy,
		SentBy:       m.SentBy,
	}
	dbService.addChatGroupMember(ctx, cgm)
}

type AddChatGroupMemberModel struct {
	Id           string `json:"id" dynamodbav:"id"`
	ChatId       string `json:"chatId" dynamodbav:"chatId"`
	MemberUserId string `json:"memberUserId" dynamodbav:"memberUserId"`
	SentBy       string `json:"sentBy" dynamodbav:"sentBy"`
}

func handleAddChatGroupMember(ctx context.Context, m AddChatGroupMemberModel) {
	//send chat group details to new member
	cg, err := dbService.getChatGroupById(ctx, m.ChatId)
	if err != nil {
		log.Printf("Couldn't fetch chat group. Here's why: %v\n", err)
	}

	cgsp := ServerPush{
		Id:     betterguid.New(),
		UserId: m.MemberUserId,
		Type:   ServerPushAddChatGroup,
		Data:   cg,
	}

	go hub.send(ctx, m.MemberUserId, cgsp, true)

	members, err := dbService.getChatGroupMembers(ctx, m.ChatId)
	if err != nil {
		log.Printf("Couldn't fetch chat group members. Here's why: %v\n", err)
	}

	//send current group members to new member
	for _, member := range members {
		sp := ServerPush{
			Id:     betterguid.New(),
			UserId: m.MemberUserId,
			Type:   ServerPushAddChatGroupMember,
			Data:   member,
		}

		go hub.send(ctx, m.MemberUserId, sp, true)
	}

	//send new group member to current members
	for _, member := range members {
		// the message sender already knows that a new member was added
		if member.MemberUserId == m.SentBy {
			continue
		}

		sp := ServerPush{
			Id:     betterguid.New(),
			UserId: member.MemberUserId,
			Type:   ServerPushAddChatGroupMember,
			Data:   m,
		}

		go hub.send(ctx, member.MemberUserId, sp, true)
	}

	//save
	dbService.addChatGroupMember(ctx, m)
}

func handleAddPresence(ctx context.Context, m AddPresenceModel) {
	//save presence
	presenceMap[m.SentBy] = m

	go hub.sendToChat(ctx, m.ChatId, m.Id, ServerPushAddPresence, m, false, m.SentBy)
}

type MessageReactionType int16

const (
	MessageReactionGoodJob MessageReactionType = iota
	MessageReactionThanks
	MessageReactionWellDone
)

type AddGoodJobMessageModel struct {
	Id        string              `json:"id" dynamodbav:"id"`
	TaskId    string              `json:"taskId" dynamodbav:"taskId"`
	ChatId    string              `json:"chatId" dynamodbav:"chatId"`
	Type      MessageReactionType `json:"type" dynamodbav:"type"`
	TaskTitle string              `json:"taskTitle" dynamodbav:"taskTitle"`
	SentTo    string              `json:"sentTo" dynamodbav:"sentTo"`
	SentBy    string              `json:"sentBy" dynamodbav:"sentBy"`
	Timestamp time.Time           `json:"timestamp" dynamodbav:"timestamp"`
}

func handleAddGoodJobMessage(ctx context.Context, m AddGoodJobMessageModel) {
	// send receipt
	mr := MessageReceiptModel{
		Type:      Sent,
		MessageId: m.Id,
		Timestamp: time.Now().UTC(),
	}

	receiptId := betterguid.New()

	pr := ServerPush{
		Id:     receiptId,
		UserId: m.SentBy,
		Type:   ServerPushMessageReceipt,
		Data:   mr,
	}

	go hub.send(ctx, m.SentBy, pr, true)

	//send done
	go hub.sendToChat(ctx, m.ChatId, m.Id, ServerPushAddGoodJobMessage, m, true, m.SentBy)

	//send notification
	notificationService.sendGoodJobNotification(ctx, m.SentBy, m.SentTo, m.TaskId, m.TaskTitle, m.Id, m.Type)
}
