package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"firebase.google.com/go/v4/messaging"
)

type DeviceToken struct {
	Uid       string    `json:"uid"`
	Token     string    `json:"token"`
	Timestamp time.Time `json:"timestamp"`
}

func loadDeviceTokensForUser(c context.Context, uid string) ([]DeviceToken, error) {
	ref := firebaseDbClient.NewRef(pathUserTokens)

	results, err := ref.Child(uid).OrderByKey().GetOrdered(c)
	if err != nil {
		log.Fatalln("Error querying database:", err)
	}

	var deviceTokens []DeviceToken
	for _, r := range results {
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

		timestamp, err := time.Parse(time.RFC3339, v)
		if err != nil {
			log.Fatalln("Cannot parse time")
		}

		deviceToken := DeviceToken{
			Uid:       uid,
			Token:     r.Key(),
			Timestamp: timestamp,
		}

		deviceTokens = append(deviceTokens, deviceToken)
	}

	return deviceTokens, nil
}

func loadDeviceTokens(c context.Context, userIdList []AppUser) []DeviceToken {
	var deviceTokens []DeviceToken

	for _, u := range userIdList {
		tokens, err := loadDeviceTokensForUser(c, u.Uid)
		if err != nil {
			log.Fatalln("Could not load tokens for " + u.Uid)
		}
		deviceTokens = append(deviceTokens, tokens...)
	}

	return deviceTokens
}

func removeFailedTokens(c context.Context, tokens []DeviceToken) {
	ref := firebaseDbClient.NewRef(pathUserTokens)
	for _, d := range tokens {
		if err := ref.Child(d.Uid).Child(d.Token).Delete(c); err != nil {
			log.Fatalln("Error removing device token:", err)
		}
	}
}

func sendNewMessageNotification(c context.Context, msg AddMessageModel, threadId string, threadName string, recepients []AppUser) {
	badgeCount := int(1)
	message := &messaging.MulticastMessage{
		Notification: &messaging.Notification{
			Title: msg.Sender,
			Body:  msg.Message.Content,
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Alert: &messaging.ApsAlert{
						SubTitle: threadName,
					},
					ThreadID:       threadId,
					MutableContent: true,
					Sound:          "default",
					Badge:          &badgeCount,
				},
			},
		},
		Data: map[string]string{
			"displayName":     "850",
			"id":              msg.Id,
			"sender":          msg.Sender,
			"senderPublicKey": msg.SenderPublicKey,
			"author":          msg.Author,
			"channelId":       msg.Channel,
			"threadId":        msg.Thread,
			"groupId":         msg.Group,
			"timestamp":       msg.Timestamp.Format(time.RFC3339),
			"isLarge":         "0",
			"type":            strconv.Itoa(int(ServerPushAddMessage)),
		},
	}

	sendNotificationToUsers(c, message, threadId, threadName, recepients)
}

func sendNewThreadNotification(c context.Context, sender string, threadId string, threadName string, recepients []AppUser) {
	message := &messaging.MulticastMessage{
		Notification: &messaging.Notification{
			Title: sender,
			Body:  "Created a new thread",
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Alert: &messaging.ApsAlert{
						SubTitle: threadName,
					},
					ThreadID:       threadId,
					MutableContent: true,
					Sound:          "default",
				},
			},
		},
		Data: map[string]string{
			"displayName": "850",
			"sender":      sender,
			"type":        strconv.Itoa(int(ServerPushAddThread)),
		},
	}

	sendNotificationToUsers(c, message, threadId, threadName, recepients)
}

func sendNewTaskNotification(c context.Context, task AddTaskModel, recepients []AppUser) {
	badgeCount := int(1)
	message := &messaging.MulticastMessage{
		Notification: &messaging.Notification{
			Title: "New Task",
			Body:  task.Title,
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					MutableContent: true,
					Sound:          "default",
					Badge:          &badgeCount,
				},
			},
		},
		Data: map[string]string{
			"isLarge": "0",
			"type":    strconv.Itoa(int(ServerPushAddTask)),
		},
	}

	sendNotificationToUsers(c, message, "", "", recepients)
}

func sendTaskCompletedNotification(c context.Context, task AddTaskModel, recepients []AppUser) {
	badgeCount := int(1)
	message := &messaging.MulticastMessage{
		Notification: &messaging.Notification{
			Title: "Task Completed",
			Body:  task.Title,
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					MutableContent: true,
					Sound:          "default",
					Badge:          &badgeCount,
				},
			},
		},
		Data: map[string]string{
			"isLarge": "0",
			"type":    strconv.Itoa(int(ServerPushAddTaskLogItem)),
		},
	}

	sendNotificationToUsers(c, message, "", "", recepients)
}

func sendTaskPendingNotification(c context.Context, task AddTaskModel, recepients []AppUser) {
	badgeCount := int(1)
	message := &messaging.MulticastMessage{
		Notification: &messaging.Notification{
			Title: "Task Pending",
			Body:  task.Title,
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					MutableContent: true,
					Sound:          "default",
					Badge:          &badgeCount,
				},
			},
		},
		Data: map[string]string{
			"isLarge": "0",
			"type":    strconv.Itoa(int(ServerPushAddTaskLogItem)),
		},
	}

	sendNotificationToUsers(c, message, "", "", recepients)
}

func sendNotificationToUsers(c context.Context, msg *messaging.MulticastMessage, threadId string, threadName string, recepients []AppUser) {
	tokenStart := time.Now()
	deviceTokens := loadDeviceTokens(c, recepients)
	tokenDuration := time.Since(tokenStart)

	// Formatted string, such as "2h3m0.5s" or "4.503μs"
	fmt.Println("Loaded device tokens", tokenDuration)
	deviceTokenCount := len(deviceTokens)

	if deviceTokenCount == 0 {
		return
	}

	if deviceTokenCount > 500 {
		log.Printf("%s : Device tokens (%v) larger than 500\n", c.Value(logPrefix), deviceTokenCount)
		for i := 0; i < deviceTokenCount/500; i++ {
			deviceTokenPart := deviceTokens[i*500 : (i+1)*500]
			sendNotification(c, msg, threadId, threadName, deviceTokenPart)
		}
		remainderDeviceTokens := deviceTokens[(deviceTokenCount/500)*500:]
		sendNotification(c, msg, threadId, threadName, remainderDeviceTokens)
	} else {
		sendNotification(c, msg, threadId, threadName, deviceTokens)
	}

	log.Printf("%s : Sent notification in thread (%s) to %v members\n", c.Value(logPrefix), threadId, len(recepients))
}

func sendNotification(c context.Context, message *messaging.MulticastMessage, threadId string, threadName string, deviceTokens []DeviceToken) {
	registrationTokens := make([]string, len(deviceTokens))
	for i, d := range deviceTokens {
		registrationTokens[i] = d.Token
	}

	message.Tokens = registrationTokens

	start := time.Now()
	br, err := cloudMessagingClient.SendMulticast(c, message)
	tokenDuration := time.Since(start)

	// Formatted string, such as "2h3m0.5s" or "4.503μs"
	fmt.Println("Notification call ", tokenDuration)
	if err != nil {
		log.Fatalln(err)
	}

	if br.FailureCount > 0 {
		var failedTokens []DeviceToken
		for idx, resp := range br.Responses {
			if !resp.Success {
				// The order of responses corresponds to the order of the registration tokens.
				failedTokens = append(failedTokens, deviceTokens[idx])
			}
		}
		removeFailedTokens(c, failedTokens)
		fmt.Printf("List of tokens that caused failures: %v\n", failedTokens)
	}
}
