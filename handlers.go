package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kjk/betterguid"
	"github.com/segmentio/kafka-go"

	"log"
)

func getUserIdFromContext(c *gin.Context) string {
	return c.Request.Header["User-Id"][0]
}

func getOffset(c *gin.Context) int64 {
	intVar, err := strconv.ParseInt(c.Request.Header["Offset"][0], 10, 64)
	if err != nil {
		return kafka.FirstOffset
	}
	return intVar
}

func syncContacts(c *gin.Context) {
	uid := c.MustGet(uidKey).(string)
	// currPhoneNumber := "+16505553434"
	// currAppUser := AppUser{Uid: uid, PhoneNumber: currPhoneNumber}
	var contacts []string

	// Call BindJSON to bind the received JSON to
	// newAlbum.
	if err := c.BindJSON(&contacts); err != nil {
		c.AbortWithError(400, err)
		return
	}
	// fmt.Println("First contact %s", contacts.Data[0])

	// var appUserSlice []AppUser
	ref := firebaseDbClient.NewRef("users")

	results, err := ref.OrderByKey().GetOrdered(c)
	if err != nil {
		log.Fatalln("Error querying database:", err)
	}

	snapshot := make([]AppUser, len(results))
	for i, r := range results {
		var d AppUser
		if err := r.Unmarshal(&d); err != nil {
			log.Fatalln("Error unmarshaling result:", err)
		}
		snapshot[i] = d
	}

	appUsers := make(map[string]AppUser)
	var currAppUser AppUser
	for _, appUser := range snapshot {
		if appUser.Uid == uid {
			currAppUser = appUser
		}
		appUsers[appUser.PhoneNumber] = appUser
	}

	appUserContacts := make([]AppUser, 0)

	for _, value := range contacts {
		if appUser, exists := appUsers[value]; exists {
			appUserContacts = append(appUserContacts, appUser)
		}
	}

	//notify contacts of new app user

	ctx := c.Copy()
	for _, appUserContact := range appUserContacts {
		serverPush := ServerPush{
			Id:   betterguid.New(),
			Type: ServerPushAppContact,
			Data: currAppUser,
		}

		go hub.send(ctx, appUserContact.Uid, serverPush, true)
	}

	c.IndentedJSON(http.StatusOK, appUserContacts)
}

type ChatChannel struct {
	ChannelId string `json:"uid"`
	Title     string `json:"title"`
	GroupId   string `json:"group"`
}

type ChatGroup struct {
	Author         string             `json:"author"`
	GroupId        string             `json:"group"`
	GroupName      string             `json:"groupName"`
	IsChat         bool               `json:"isChat"`
	DefaultChannel ChatChannel        `json:"defaultChannel"`
	Members        map[string]AppUser `json:"members"`
}

type ServerPushType int16

const (
	ServerPushAddGroup             ServerPushType = iota //0
	ServerPushAddThread                                  //1
	ServerPushAddMessage                                 //2
	ServerPushMessageReceipt                             //3
	ServerPushChangeGroupName                            //4
	ServerPushAddGroupMember                             //5
	ServerPushRemoveGroupMember                          //6
	ServerPushSystemMessage                              //7
	ServerPushClientReply                                //8
	ServerPushAddChatId                                  //9
	ServerPushAppContact                                 //10
	ServerPushUpdateThreadTitle                          //11
	ServerPushAddTaskLogItem                             //12
	ServerPushAddTask                                    //13
	ServerPushAddTaskStatus                              //14
	ServerPushAddTaskMessage                             //15
	ServerPushAddChat                                    //16
	ServerPushAddChatMessage                             //17
	ServerPushAddTaskReminder                            //18
	ServerPushAddTaskDone                                //19
	ServerPushAddTaskNotDone                             //20
	ServerPushAddWaitingRequest                          //21
	ServerPushAcceptWaitingRequest                       //22
	ServerPushDenyWaitingRequest                         //23
	ServerPushAddChatGroup                               //24
	ServerPushAddChatGroupMember                         //25
	ServerPushAddPresence                                //26
	ServerPushAddGoodJobMessage                          //27
)

type ServerPush struct {
	Id     string         `json:"id"  dynamodbav:"id"`
	UserId string         `json:"userId"  dynamodbav:"userId"`
	Offset int64          `json:"offset"`
	Type   ServerPushType `json:"type"  dynamodbav:"type"`
	Data   interface{}    `json:"data"  dynamodbav:"data"`
}

const pathUsers string = "users"
const pathGroups string = "groups"
const pathUserGroups string = "test_userGroups"
const pathUserChatIds string = "userChatIDs"
const pathMessages string = "test_messages"
const pathUserMessages string = "userMessages"
const pathMessageReceipts string = "messageReceipts"
const pathThreads string = "threads"
const pathUserThreads string = "test_userThreads"
const pathUserTokens string = "userTokens"
const pathWorkspaces string = "workspaces"

func findChatPartner(currMember AppUser, members map[string]AppUser) (*AppUser, error) {
	for _, member := range members {
		if member.PhoneNumber != currMember.PhoneNumber {
			return &member, nil
		}
	}

	return nil, errors.New("no partner found")
}

func addGroup(ctx *gin.Context) {
	var chatGroup ChatGroup

	if err := ctx.BindJSON(&chatGroup); err != nil {
		ctx.AbortWithError(400, err)
		return
	}

	groupsRef := firebaseDbClient.NewRef(pathGroups)
	err := groupsRef.Child(chatGroup.GroupId).Set(ctx, chatGroup)
	if err != nil {
		log.Fatalln("Error setting value:", err)
	}

	ctx.IndentedJSON(http.StatusOK, gin.H{"status": "ok"})

	c := ctx.Copy()
	go func() {
		userGroupUpdates := make(map[string]interface{})

		for _, member := range chatGroup.Members {
			userChatGroup := chatGroup

			if userChatGroup.IsChat {
				partner, _ := findChatPartner(member, userChatGroup.Members)
				userChatGroup.GroupName = partner.PhoneNumber
			}

			userGroupUpdates["/"+member.Uid+"/"+userChatGroup.GroupId] = userChatGroup
		}

		userGroupsRef := firebaseDbClient.NewRef(pathUserGroups)

		if err := userGroupsRef.Update(c, userGroupUpdates); err != nil {
			log.Fatalln("Error updating children:", err)
		}

		if chatGroup.IsChat {
			childUpdates := make(map[string]interface{})

			for _, member := range chatGroup.Members {
				partner, _ := findChatPartner(member, chatGroup.Members)
				childUpdates["/"+member.Uid+"/"+partner.PhoneNumber] = chatGroup.GroupId
			}

			userChatIdsRef := firebaseDbClient.NewRef(pathUserChatIds)

			if err := userChatIdsRef.Update(c, childUpdates); err != nil {
				log.Fatalln("Error updating children:", err)
			}
		}
	}()
}

type Mention struct {
	Id          string `json:"id"`
	Uid         string `json:"uid"`
	PhoneNumber string `json:"phoneNumber"`
	Range       [2]int `json:"range"`
}

type TaskMention struct {
	Id    string `json:"id"`
	Uid   string `json:"uid"`
	Title string `json:"title"`
	Range [2]int `json:"range"`
}

type Message struct {
	Content          string        `json:"content,omitempty"`
	Mentions         []Mention     `json:"mentions"`
	TaskMentions     []TaskMention `json:"taskMentions"`
	Links            []string      `json:"links"`
	ImageDownloadUrl string        `json:"imageDownloadUrl,omitempty"`
	ImageBlurHash    string        `json:"imageBlurHash,omitempty"`
}

type ChatMessage struct {
	Id              string    `json:"id"`
	Author          string    `json:"author"`
	Sender          string    `json:"sender"`
	Timestamp       time.Time `json:"timestamp"`
	Channel         string    `json:"channel"`
	Group           string    `json:"group"`
	ChannelName     string    `json:"channelName"`
	GroupName       string    `json:"groupName"`
	Message         Message   `json:"message"`
	IsChat          bool      `json:"isChat"`
	IsSystemMessage bool      `json:"isSystemMessage"`
	//threads
	Thread          string `json:"thread,omitempty"`
	ThreadName      string `json:"threadName,omitempty"`
	IsThreadMessage bool   `json:"isThreadMessage"`
	// replies
	ReplyCount           int16  `json:"replyCount"`
	LatestReplyMessageId string `json:"latestReplyMessageId,omitempty"`
	// ReplyingToMessageId   string `json:"replyOriginalMessage,omitempty"`
	IsSent                bool `json:"isSent"`
	IsDelivered           bool `json:"isDelivered"`
	IsRead                bool `json:"isRead"`
	IsReadByCurrUser      bool `json:"isReadByCurrUser"`
	IsDeliveredToCurrUser bool `json:"isDeliveredToCurrUser"`
}

func addMessage(ctx *gin.Context) {
	var chatMessage ChatMessage

	if err := ctx.BindJSON(&chatMessage); err != nil {
		ctx.AbortWithError(400, err)
		return
	}

	cCp := ctx.Copy()

	go func() {
		// chatMessage.Timestamp = time.Now().UTC()
		chatMessage.IsSent = true

		messagesRef := firebaseDbClient.NewRef(pathMessages)
		err := messagesRef.Child(chatMessage.Id).Set(cCp, chatMessage)
		if err != nil {
			log.Fatalln("Error setting value:", err)
		}

		var chatGroup ChatGroup
		groupsRef := firebaseDbClient.NewRef(pathGroups)
		if err := groupsRef.Child(chatMessage.Group).Get(cCp, &chatGroup); err != nil {
			log.Fatalln("Error reading value for chatGroup:", err)
		}

		childUpdates := make(map[string]interface{})

		for _, member := range chatGroup.Members {
			childUpdates["/"+member.Uid+"/"+chatMessage.Id] = chatMessage
		}

		userMessagesRef := firebaseDbClient.NewRef(pathUserMessages)

		if err := userMessagesRef.Update(cCp, childUpdates); err != nil {
			log.Fatalln("Error updating children userMessagesRef:", err)
		}

		receiptUpdates := make(map[string]interface{})

		receiptUpdates["/"+chatMessage.Id+"/message"] = chatMessage
		receiptUpdates["/"+chatMessage.Id+"/isSent"] = true

		receiptsRef := firebaseDbClient.NewRef(pathMessageReceipts)

		if err := receiptsRef.Update(cCp, receiptUpdates); err != nil {
			log.Fatalln("Error updating children receiptsRef:", err)
		}
	}()

	ctx.IndentedJSON(http.StatusOK, gin.H{"status": "ok"})
}

type MessageReceipt struct {
	Message   ChatMessage        `json:"message"`
	IsSent    bool               `json:"isSent"`
	Delivered map[string]AppUser `json:"delivered"`
	Read      map[string]AppUser `json:"read"`
}

func addDeliveredReceipt(ctx *gin.Context) {
	uid := ctx.MustGet(uidKey).(string)

	var ack AddAckModel
	if err := ctx.BindJSON(&ack); err != nil {
		ctx.AbortWithError(400, err)
		return
	}

	ctx.IndentedJSON(http.StatusOK, gin.H{"status": "ok"})

	c := ctx.Copy()
	go func() {
		var currAppUser AppUser

		usersRef := firebaseDbClient.NewRef(pathUsers)
		if err := usersRef.Child(uid).Get(c, &currAppUser); err != nil {
			log.Fatalln("Error reading value:", err)
		}

		userMessagesRef := firebaseDbClient.NewRef(pathUserMessages)

		var pushMessage ServerPush
		if err := userMessagesRef.Child(uid).Child(ack.MessageId).Get(c, &pushMessage); err != nil {
			log.Fatalln("Error reading value:", err)
		}

		//delete message after delivery is confirmed
		if err := userMessagesRef.Child(uid).Child(ack.MessageId).Delete(ctx); err != nil {
			log.Fatalln("Error removing delivered message:", err)
		}

		//send delivered receipt to message author
		if pushMessage.Type == ServerPushAddMessage {
			jsonString, _ := json.Marshal(pushMessage.Data)
			var message AddMessageModel
			err := json.Unmarshal(jsonString, &message)
			if err != nil {
				log.Println("Could not convert data to message")
			}
			messageReceipt := MessageReceiptModel{
				Type:      Delivered,
				MessageId: ack.MessageId,
				AppUser:   currAppUser.Uid,
			}

			receiptId := betterguid.New()

			serverPushReceipt := ServerPush{
				Id:   receiptId,
				Type: ServerPushMessageReceipt,
				Data: messageReceipt,
			}

			go hub.send(c, message.Author, serverPushReceipt, true)
		}
	}()

}

type AddReadReceiptModel struct {
	Receipts []ReadReceipt `json:"receipts"`
}

type ReadReceipt struct {
	Author    string `json:"author"`
	MessageId string `json:"messageId"`
}

func addReadReceipt(ctx *gin.Context) {
	uid := getUserIdFromContext(ctx)

	var receipts AddReadReceiptModel

	if err := ctx.BindJSON(&receipts); err != nil {
		ctx.AbortWithError(400, err)
		return
	}

	cCp := ctx.Copy()
	go func() {
		usersRef := firebaseDbClient.NewRef(pathUsers)

		var currAppUser AppUser
		if err := usersRef.Child(uid).Get(cCp, &currAppUser); err != nil {
			log.Fatalln("Error reading value:", err)
		}

		for _, receipt := range receipts.Receipts {
			messageReceipt := MessageReceiptModel{
				Type:      Read,
				MessageId: receipt.MessageId,
				AppUser:   currAppUser.Uid,
			}

			receiptId := betterguid.New()

			serverPushReceipt := ServerPush{
				Id:   receiptId,
				Type: ServerPushMessageReceipt,
				Data: messageReceipt,
			}

			go hub.send(cCp, receipt.Author, serverPushReceipt, true)
		}
	}()

	ctx.IndentedJSON(http.StatusOK, gin.H{"status": "ok"})
}

func updateUserMessages(c context.Context, messageId string) {
	var messageReceipt MessageReceipt

	receiptsRef := firebaseDbClient.NewRef(pathMessageReceipts)

	if err := receiptsRef.Child(messageId).Get(c, &messageReceipt); err != nil {
		log.Fatalln("Error reading value:", err)
	}

	var chatGroup ChatGroup
	groupsRef := firebaseDbClient.NewRef(pathGroups)
	if err := groupsRef.Child(messageReceipt.Message.Group).Get(c, &chatGroup); err != nil {
		log.Fatalln("Error reading value:", err)
	}

	// memberCount := len(chatGroup.Members)
	var recepients []AppUser

	for _, member := range chatGroup.Members {
		recepients = append(recepients, member)
	}

	var isDelivered bool = true
	for _, recepient := range recepients {
		deliveredTo := messageReceipt.Delivered
		if _, exists := deliveredTo[recepient.Uid]; !exists {
			isDelivered = false
		}
	}

	var isRead bool = false
	for _, recepient := range recepients {
		readBy := messageReceipt.Read
		if _, exists := readBy[recepient.Uid]; exists {
			isRead = true
			break
		}
	}

	userMessageUpdates := make(map[string]interface{})

	userMessageUpdates["/"+messageReceipt.Message.Author+"/"+messageId+"/isSent"] = true
	userMessageUpdates["/"+messageReceipt.Message.Author+"/"+messageId+"/isDelivered"] = isDelivered
	userMessageUpdates["/"+messageReceipt.Message.Author+"/"+messageId+"/isRead"] = isRead

	userMessagesRef := firebaseDbClient.NewRef(pathUserMessages)

	if err := userMessagesRef.Update(c, userMessageUpdates); err != nil {
		log.Fatalln("Error updating children:", err)
	}
}

type ChatThread struct {
	Author    string             `json:"author"`
	Uid       string             `json:"uid"`
	GroupId   string             `json:"group"`
	ChannelId string             `json:"channel"`
	Title     Message            `json:"title"`
	Members   map[string]AppUser `json:"members"`
}

func addThread(ctx *gin.Context) {
	var chatThread ChatThread

	if err := ctx.BindJSON(&chatThread); err != nil {
		ctx.AbortWithError(400, err)
		return
	}

	threadsRef := firebaseDbClient.NewRef(pathThreads)
	err := threadsRef.Child(chatThread.Uid).Set(ctx, chatThread)
	if err != nil {
		log.Fatalln("Error setting value:", err)
	}

	cCp := ctx.Copy()
	go func() {
		userThreadUpdates := make(map[string]interface{})

		for _, member := range chatThread.Members {
			userThreadUpdates["/"+member.Uid+"/"+chatThread.Uid] = chatThread
		}

		userThreadsRef := firebaseDbClient.NewRef(pathUserThreads)

		if err := userThreadsRef.Update(cCp, userThreadUpdates); err != nil {
			log.Fatalln("Error updating children:", err)
		}
	}()

	ctx.IndentedJSON(http.StatusOK, gin.H{"status": "ok"})
}

type ClientPushType int16

const (
	ClientPushAddGroup             ClientPushType = 1
	ClientPushAddThread            ClientPushType = 2
	ClientPushAddMessage           ClientPushType = 3
	ClientPushAck                  ClientPushType = 4
	ClientPushPing                 ClientPushType = 5
	ClientPushReadReceipt          ClientPushType = 6
	ClientPushAddTaskLogItem       ClientPushType = 7
	ClientPushAddTask              ClientPushType = 8
	ClientPushAddTaskStatus        ClientPushType = 9
	ClientPushAddTaskMessage       ClientPushType = 10
	ClientPushAddChat              ClientPushType = 11
	ClientPushAddChatMessage       ClientPushType = 12
	ClientPushAddTaskReminder      ClientPushType = 13
	ClientPushAddTaskDone          ClientPushType = 14
	ClientPushAddTaskNotDone       ClientPushType = 15
	ClientPushAddWaitingRequest    ClientPushType = 16
	ClientPushAcceptWaitingRequest ClientPushType = 17
	ClientPushDenytWaitingRequest  ClientPushType = 18
	ClientPushAddChatGroup         ClientPushType = 19
	ClientPushAddChatGroupMember   ClientPushType = 20
	ClientPushAddPresence          ClientPushType = 21
	ClientPushAddGoodJobMessage    ClientPushType = 22
)

type ClientPush struct {
	Id   uint32         `json:"id"`
	Type ClientPushType `json:"type"`
	Data interface{}    `json:"data"`
}

func handlePush(ctx *gin.Context) {
	var clientPush ClientPush

	if err := ctx.BindJSON(&clientPush); err != nil {
		ctx.AbortWithError(400, err)
		return
	}

	// convert map to json
	jsonString, _ := json.Marshal(clientPush.Data)
	fmt.Println(string(jsonString))

	// convert json to struct
	s := ChatThread{}
	json.Unmarshal(jsonString, &s)
	fmt.Println(s.Title.Content)

	// log.Println(clientPush.Data.(string))

	ctx.IndentedJSON(http.StatusOK, gin.H{"status": "ok"})
}

type ChatId struct {
	GroupId     string `json:"groupId"`
	PhoneNumber string `json:"phoneNumber"`
}

func getChatIds(c *gin.Context) {
	uid := c.MustGet(uidKey).(string)
	// fmt.Println("First contact %s", contacts.Data[0])

	// var appUserSlice []AppUser
	ref := firebaseDbClient.NewRef(pathUserChatIds)

	results, err := ref.Child(uid).OrderByKey().GetOrdered(c)
	if err != nil {
		log.Fatalln("Error querying database:", err)
	}

	chatIds := make([]ChatId, len(results))
	for i, r := range results {
		var v string
		if err := r.Unmarshal(&v); err != nil {
			log.Fatalln("Error unmarshaling result:", err)
		}
		// for k, v := range d {
		// 	chatId := ChatId{
		// 		PhoneNumber: k,
		// 		GroupId:     v,
		// 	}
		// 	chatIds[i] = chatId
		// }
		chatId := ChatId{
			PhoneNumber: r.Key(),
			GroupId:     v,
		}
		chatIds[i] = chatId
	}

	c.IndentedJSON(http.StatusOK, gin.H{"chatIds": chatIds})
}

type AddUserModel struct {
	Uid         string `json:"uid" dynamodbav:"userId"`
	PhoneNumber string `json:"phoneNumber" dynamodbav:"phoneNumber"`
	DisplayName string `json:"displayName" dynamodbav:"displayName"`
	PublicKey   string `json:"publicKey" dynamodbav:"publicKey"`
	FirstName   string `json:"firstName" dynamodbav:"firstName"`
	LastName    string `json:"lastName" dynamodbav:"lastName"`
}

type AddWorkspaceModel struct {
	Uid     string             `json:"uid"`
	Title   string             `json:"title"`
	Members map[string]AppUser `json:"members"`
}

func addUser(ctx *gin.Context) {
	uid := ctx.MustGet(uidKey).(string)

	var user AddUserModel
	if err := ctx.BindJSON(&user); err != nil {
		ctx.AbortWithError(400, err)
		return
	}

	err := dbService.addUser(ctx, user)
	if err != nil {
		log.Fatalln("Error setting value:", err)
	}

	members, err := dbService.getUsers(ctx)
	if err != nil {
		log.Fatalln("Error scanning users:", err)
	}

	c := ctx.Copy()

	for _, member := range members {
		if member.Uid == uid {
			continue
		}

		appUser := AppUser{
			Uid:       member.Uid,
			FirstName: member.FirstName,
			LastName:  member.LastName,
		}

		serverPush := ServerPush{
			UserId: member.Uid,
			Id:     betterguid.New(),
			Type:   ServerPushAppContact,
			Data:   appUser,
		}

		go hub.send(c, member.Uid, serverPush, true)
	}

	ctx.IndentedJSON(http.StatusOK, user)
}

func addDemoWorkspace(c *gin.Context) {
	uid := c.MustGet(uidKey).(string)

	var workspace AddWorkspaceModel
	if err := c.BindJSON(&workspace); err != nil {
		c.AbortWithError(400, err)
		return
	}

	usersRef := firebaseDbClient.NewRef(pathUsers)

	var currAppUser AppUser
	if err := usersRef.Child(uid).Get(c, &currAppUser); err != nil {
		log.Fatalln("Error reading value:", err)
	}

	var defaultUser1 AppUser
	if err := usersRef.Child("dEhiDlpBWuNzA9VZPxGKd9namwl1").Get(c, &defaultUser1); err != nil {
		log.Fatalln("Error reading value:", err)
	}

	var defaultUser2 AppUser
	if err := usersRef.Child("sfHopUEFKoWU8qUVapVAWwfqN0i1").Get(c, &defaultUser2); err != nil {
		log.Fatalln("Error reading value:", err)
	}

	defaultGroupUid := betterguid.New()

	chatGroup := ChatGroup{
		Author:    uid,
		GroupId:   defaultGroupUid,
		GroupName: workspace.Title,
		DefaultChannel: ChatChannel{
			ChannelId: betterguid.New(),
			Title:     "General",
			GroupId:   defaultGroupUid,
		},
		IsChat: false,
		Members: map[string]AppUser{
			uid:              currAppUser,
			defaultUser1.Uid: defaultUser1,
			defaultUser2.Uid: defaultUser2,
		},
	}

	groupsRef := firebaseDbClient.NewRef(pathGroups)

	ctx := context.WithValue(c, logPrefix, uid+"/addDemoWorkspace")

	err := groupsRef.Child(chatGroup.GroupId).Set(ctx, chatGroup)
	if err != nil {
		log.Fatalln("Error setting value:", err)
	}

	// guid := betterguid.New()

	for _, member := range chatGroup.Members {
		if member.Uid == chatGroup.Author {
			continue
		}

		serverPush := ServerPush{
			Id:   chatGroup.GroupId,
			Type: ServerPushAddGroup,
			Data: chatGroup,
		}

		hub.send(ctx, member.Uid, serverPush, true)
	}

	log.Printf("%s : Sent addGroup message to %v members\n", ctx.Value(logPrefix), len(chatGroup.Members))

	c.IndentedJSON(http.StatusOK, chatGroup)
}

type AddTokenModel struct {
	UserId    string    `json:"userId" dynamodbav:"userId"`
	Token     string    `json:"token" dynamodbav:"token"`
	Timestamp time.Time `json:"timestamp" dynamodbav:"timestamp"`
	Debug     bool      `json:"debug" dynamodbav:"debug"`
}

func addToken(c *gin.Context) {
	// uid := c.MustGet(uidKey).(string)

	var token AddTokenModel
	if err := c.BindJSON(&token); err != nil {
		c.AbortWithError(400, err)
		return
	}

	err := dbService.addDeviceToken(c, token)
	if err != nil {
		log.Fatalln("Error setting value:", err)
	}

	c.IndentedJSON(http.StatusOK, gin.H{"status": "ok"})
}

func getMessages(c *gin.Context) {
	uid := c.MustGet(uidKey).(string)

	userMessagesRef := firebaseDbClient.NewRef(pathUserMessages)

	results, err := userMessagesRef.Child(uid).OrderByKey().GetOrdered(c)
	if err != nil {
		log.Fatalln("Error querying database:", err)
	}
	snapshot := make([]ServerPush, len(results))
	for i, r := range results {
		var m ServerPush
		if err := r.Unmarshal(&m); err != nil {
			log.Fatalln("Error unmarshaling result:", err)
		}
		snapshot[i] = m
	}

	c.IndentedJSON(http.StatusOK, gin.H{"data": snapshot})
}

type UpdateThreadTitleModel struct {
	ThreadId string `json:"threadId"`
	Title    string `json:"title"`
}

func updateThreadTitle(ctx *gin.Context) {
	// threadId := ctx.Param("threadId")
	uid := ctx.MustGet(uidKey).(string)

	var model UpdateThreadTitleModel
	if err := ctx.BindJSON(&model); err != nil {
		ctx.AbortWithError(400, err)
		return
	}

	ctx.IndentedJSON(http.StatusOK, gin.H{"status": "ok"})

	c := ctx.Copy()
	go func() {
		threadsRef := firebaseDbClient.NewRef(pathThreads)

		var chatThread ChatThread
		if err := threadsRef.Child(model.ThreadId).Get(c, &chatThread); err != nil {
			log.Fatalln("Error reading value for ChatThread: "+model.ThreadId, err)
		}

		chatThread.Title.Content = model.Title
		err := threadsRef.Child(model.ThreadId).Set(c, chatThread)
		if err != nil {
			log.Fatalln("Error setting value:", err)
		}

		var members []AppUser

		for _, member := range chatThread.Members {
			if member.Uid == uid {
				continue
			}
			members = append(members, member)
		}

		for _, member := range members {
			serverPush := ServerPush{
				Id:   betterguid.New(),
				Type: ServerPushUpdateThreadTitle,
				Data: model,
			}

			hub.send(ctx, member.Uid, serverPush, true)
		}

		var currAppUser AppUser

		usersRef := firebaseDbClient.NewRef(pathUsers)
		if err := usersRef.Child(uid).Get(c, &currAppUser); err != nil {
			log.Fatalln("Error reading value:", err)
		}

		systemMessage := AddSystemMessageModel{
			Type:      SystemMessageUpdateThreadTitle,
			Id:        betterguid.New(),
			Timestamp: time.Now().UTC(),
			Thread:    model.ThreadId,
			Group:     chatThread.GroupId,
			Message: Message{
				Content: "@" + currAppUser.PhoneNumber + " changed topic name to \"" + model.Title + "\"",
				Mentions: []Mention{{
					Id:          betterguid.New(),
					Uid:         uid,
					PhoneNumber: currAppUser.PhoneNumber,
					Range:       [2]int{0, len(currAppUser.PhoneNumber) + 1},
				}},
				Links: make([]string, 0),
			},
			Context: SystemMessageAddThreadContext{
				ThreadUid: model.ThreadId,
			},
		}

		for _, member := range chatThread.Members {
			serverPushSystemMessage := ServerPush{
				Id:   systemMessage.Id,
				Type: ServerPushSystemMessage,
				Data: systemMessage,
			}

			hub.send(ctx, member.Uid, serverPushSystemMessage, true)
		}
	}()

}

func deleteAccount(c *gin.Context) {
	uid := c.MustGet(uidKey).(string)

	userTokensRef := firebaseDbClient.NewRef(pathUserTokens)

	//delete message after delivery is confirmed
	if err := userTokensRef.Child(uid).Delete(c); err != nil {
		log.Fatalln("Error removing user tokens:", err)
	}

	// remove user from groups

	// remove user from threads

	// remove user
	usersRef := firebaseDbClient.NewRef(pathUsers)

	if err := usersRef.Child(uid).Delete(c); err != nil {
		log.Fatalln("Error removing user:", err)
	}

	c.IndentedJSON(http.StatusOK, gin.H{"status": "ok"})
}

type WorkSpaceMember struct {
	Uid         string `json:"uid"`
	DisplayName string `json:"displayName"`
	PublicKey   string `json:"publicKey"`
}

func getWorkspaceMembers(c *gin.Context) {
	// uid := c.MustGet(uidKey).(string)

	usersRef := firebaseDbClient.NewRef(pathUsers)

	results, err := usersRef.OrderByKey().GetOrdered(c)
	if err != nil {
		log.Fatalln("Error querying database:", err)
	}
	snapshot := make([]WorkSpaceMember, len(results))
	for i, r := range results {
		var m WorkSpaceMember
		if err := r.Unmarshal(&m); err != nil {
			log.Fatalln("Error unmarshaling result:", err)
		}
		snapshot[i] = m
	}

	c.IndentedJSON(http.StatusOK, gin.H{"data": snapshot})
}

func getUsers(c *gin.Context) {
	// uid := c.MustGet(uidKey).(string)

	members, err := dbService.getUsers(c)
	if err != nil {
		log.Fatalln("Error querying database:", err)
	}

	users := make([]AppUser, len(members))
	for i, m := range members {
		users[i] = AppUser{
			Uid:       m.Uid,
			FirstName: m.FirstName,
			LastName:  m.LastName,
		}
	}

	c.IndentedJSON(http.StatusOK, gin.H{"data": users})
}

func getUserById(ctx *gin.Context) {
	userId := ctx.Param("userId")
	// uid := ctx.MustGet(uidKey).(string)

	user, err := dbService.getUserById(ctx, userId)
	if err != nil {
		log.Fatalln("Error querying database:", err)
	}
	ctx.IndentedJSON(http.StatusOK, gin.H{"data": user})

}

func getMessageByUserId(ctx *gin.Context) {
	uid := ctx.Param("userId")
	mid := ctx.Param("messageId")
	// uid := ctx.MustGet(uidKey).(string)

	messages, err := dbService.getMessageById(ctx, mid, uid)
	if err != nil {
		log.Fatalln("Error querying database:", err)
	}
	ctx.IndentedJSON(http.StatusOK, gin.H{"data": messages})

}

func ackMessage(ctx *gin.Context) {
	uid := ctx.Param("userId")
	mid := ctx.Param("messageId")

	ctx.IndentedJSON(http.StatusOK, gin.H{"status": "ok"})

	c := ctx.Copy()
	go func() {

		pm, err := dbService.getMessageById(c, mid, uid)
		if err != nil {
			log.Println("Failed to fetch message:", err)
			return
		}

		//don't delete message after delivery is confirmed for notifications

		// if err := dbService.removeMessageById(c, mid, uid); err != nil {
		// 	log.Fatalln("Error removing delivered message:", err)
		// }

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
	}()

}

type AddPresenceModel struct {
	Id        string    `json:"id" dynamodbav:"id"`
	IsPresent bool      `json:"isPresent" dynamodbav:"isPresent"`
	ChatId    string    `json:"chatId" dynamodbav:"chatId"`
	TaskId    string    `json:"taskId" dynamodbav:"taskId"`
	IsTyping  bool      `json:"isTyping" dynamodbav:"isTyping"`
	SentBy    string    `json:"sentBy" dynamodbav:"sentBy"`
	Timestamp time.Time `json:"timestamp" dynamodbav:"timestamp"`
}

func getUserPresenceById(ctx *gin.Context) {
	uid := ctx.MustGet(uidKey).(string)
	pid := ctx.Param("peerId")

	presence := presenceMap[pid]
	presenceSubscribers[pid] = append(presenceSubscribers[pid], uid)
	ctx.IndentedJSON(http.StatusOK, gin.H{"data": presence})

}

func addTask(c *gin.Context) {
	// uid := c.MustGet(uidKey).(string)

	var task AddTaskModel
	if err := c.BindJSON(&task); err != nil {
		c.AbortWithError(400, err)
		return
	}

	ctx := c.Copy()
	//save task to db
	dbService.addTask(ctx, task)

	//send task
	go hub.sendToChat(ctx, task.GroupUid, task.Id, ServerPushAddTask, task, true, task.AssignedBy)

	go notificationService.sendNewTaskNotification(ctx, task, task.Id)

	c.IndentedJSON(http.StatusOK, task)
}

func addChatGroup(c *gin.Context) {
	// uid := c.MustGet(uidKey).(string)

	var m AddChatGroupModel
	if err := c.BindJSON(&m); err != nil {
		c.AbortWithError(400, err)
		return
	}

	ctx := c.Copy()
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

	c.IndentedJSON(http.StatusOK, m)
}

func addChatGroupMember(c *gin.Context) {
	// uid := c.MustGet(uidKey).(string)

	var m AddChatGroupMemberModel
	if err := c.BindJSON(&m); err != nil {
		c.AbortWithError(400, err)
		return
	}

	ctx := c.Copy()
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

	c.IndentedJSON(http.StatusOK, m)
}

func addChat(c *gin.Context) {
	// uid := c.MustGet(uidKey).(string)

	var m AddChatModel
	if err := c.BindJSON(&m); err != nil {
		c.AbortWithError(400, err)
		return
	}

	ctx := c.Copy()

	//send new chat to partner
	pm := ServerPush{
		Id:     betterguid.New(),
		UserId: m.SentTo,
		Type:   ServerPushAddChat,
		Data:   m,
	}

	//save one to one chat as group chat
	cg := AddChatGroupModel{
		Id:        m.Id,
		Title:     "onetoone",
		SentBy:    m.SentBy,
		CreatedAt: m.CreatedAt,
	}

	dbService.addChatGroup(ctx, cg)

	//save sender as chat group member
	ogm := AddChatGroupMemberModel{
		Id:           betterguid.New(),
		ChatId:       m.Id,
		MemberUserId: m.SentBy,
		SentBy:       m.SentBy,
	}

	dbService.addChatGroupMember(ctx, ogm)

	//save chat partner as chat group member
	pgm := AddChatGroupMemberModel{
		Id:           betterguid.New(),
		ChatId:       m.Id,
		MemberUserId: m.SentTo,
		SentBy:       m.SentBy,
	}

	dbService.addChatGroupMember(ctx, pgm)

	//send chat to partner
	hub.send(ctx, m.SentTo, pm, true)

	c.IndentedJSON(http.StatusOK, m)
}
