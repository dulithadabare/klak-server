package main

import (
	"context"
	"fmt"
	"log"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/payload"
)

type NotificationService struct {
	apnsClient *apns2.Client
}

func (ns NotificationService) loadDeviceTokens(c context.Context, userIdList []string) []AddTokenModel {
	var deviceTokens []AddTokenModel

	for _, u := range userIdList {
		tokens, err := dbService.getDeviceTokens(c, u)
		if err != nil {
			log.Fatalln("Could not load tokens for " + u)
		}
		deviceTokens = append(deviceTokens, tokens...)
	}

	return deviceTokens
}

func (ns NotificationService) sendNewTaskNotification(ctx context.Context, task AddTaskModel, mid string) {
	sender, err := dbService.getUserById(ctx, task.AssignedBy)

	if err != nil {
		log.Println("Failed to fetch sender", task.AssignedBy, err)
		return
	}
	alertTitle := sender.FirstName + " " + sender.LastName
	var subtitle string

	if task.IsUrgent {
		subtitle = "(Urgent) " + task.Title
	} else {
		subtitle = task.Title
	}
	payload := payload.NewPayload().AlertTitle(alertTitle).AlertSubtitle(subtitle).AlertBody(task.Description).ThreadID(task.Id).Badge(1).Sound("default").MutableContent().Custom("mid", mid).Custom("uid", task.AssginedTo)
	tokens := ns.loadDeviceTokens(ctx, []string{task.AssginedTo})
	ns.sendNotification(ctx, payload, tokens)
}

func (ns NotificationService) sendTaskDoneNotification(ctx context.Context, sentBy string, sentTo string, taskId string, taskTitle string, mid string) {
	sender, err := dbService.getUserById(ctx, sentBy)
	if err != nil {
		log.Println("Failed to fetch sender", sentBy, err)
		return
	}

	alertTitle := sender.FirstName + " " + sender.LastName
	payload := payload.NewPayload().AlertTitle(alertTitle).AlertSubtitle(taskTitle).AlertBody("Done").ThreadID(taskId).Badge(1).Sound("default").MutableContent().Custom("mid", mid).Custom("uid", sentTo)
	tokens := ns.loadDeviceTokens(ctx, []string{sentTo})
	ns.sendNotification(ctx, payload, tokens)

}

func (ns NotificationService) sendTaskNotDoneNotification(ctx context.Context, sentBy string, sentTo string, taskId string, taskTitle string, mid string) {
	sender, err := dbService.getUserById(ctx, sentBy)
	if err != nil {
		log.Println("Failed to fetch sender", sentBy, err)
		return
	}

	alertTitle := sender.FirstName + " " + sender.LastName
	payload := payload.NewPayload().AlertTitle(alertTitle).AlertSubtitle(taskTitle).AlertBody("Not Done").ThreadID(taskId).Badge(1).Sound("default").MutableContent().Custom("mid", mid).Custom("uid", sentTo)
	tokens := ns.loadDeviceTokens(ctx, []string{sentTo})
	ns.sendNotification(ctx, payload, tokens)

}

func (ns NotificationService) sendTaskTextMessageNotification(ctx context.Context, message string, sentBy string, sentTo string, taskId string, taskTitle string, mid string) {
	sender, err := dbService.getUserById(ctx, sentBy)
	if err != nil {
		log.Println("Failed to fetch sender", sentBy, err)
		return
	}

	alertTitle := sender.FirstName + " " + sender.LastName
	payload := payload.NewPayload().AlertTitle(alertTitle).AlertSubtitle(taskTitle).AlertBody(message).ThreadID(taskId).Badge(1).Sound("default").MutableContent().Custom("mid", mid).Custom("uid", sentTo)
	tokens := ns.loadDeviceTokens(ctx, []string{sentTo})
	ns.sendNotification(ctx, payload, tokens)
}

func (ns NotificationService) sendTaskReminderNotification(ctx context.Context, sentBy string, sentTo string, taskId string, taskTitle string, mid string) {
	sender, err := dbService.getUserById(ctx, sentBy)
	if err != nil {
		log.Println("Failed to fetch sender", sentBy, err)
		return
	}

	alertTitle := sender.FirstName + " " + sender.LastName

	payload := payload.NewPayload().AlertTitle(alertTitle).AlertSubtitle(taskTitle).AlertBody("Reminder").ThreadID(taskId).Badge(1).Sound("default").MutableContent().Custom("mid", mid).Custom("uid", sentTo)
	tokens := ns.loadDeviceTokens(ctx, []string{sentTo})
	ns.sendNotification(ctx, payload, tokens)
}

func (ns NotificationService) sendTaskWaitingRequestNotification(ctx context.Context, sentBy string, sentTo string, taskId string, taskTitle string, mid string) {
	sender, err := dbService.getUserById(ctx, sentBy)
	if err != nil {
		log.Println("Failed to fetch sender", sentBy, err)
		return
	}

	alertTitle := sender.FirstName + " " + sender.LastName

	payload := payload.NewPayload().AlertTitle(alertTitle).AlertSubtitle(taskTitle).AlertBody("Waiting Request").ThreadID(taskId).Badge(1).Sound("default").MutableContent().Custom("mid", mid).Custom("uid", sentTo)
	tokens := ns.loadDeviceTokens(ctx, []string{sentTo})
	ns.sendNotification(ctx, payload, tokens)
}

func (ns NotificationService) sendTaskAcceptWaitingRequestNotification(ctx context.Context, sentBy string, sentTo string, taskId string, taskTitle string, mid string) {
	sender, err := dbService.getUserById(ctx, sentBy)
	if err != nil {
		log.Println("Failed to fetch sender", sentBy, err)
		return
	}

	alertTitle := sender.FirstName + " " + sender.LastName

	payload := payload.NewPayload().AlertTitle(alertTitle).AlertSubtitle(taskTitle).AlertBody("Waiting Request Accepted").ThreadID(taskId).Badge(1).Sound("default").MutableContent().Custom("mid", mid).Custom("uid", sentTo)
	tokens := ns.loadDeviceTokens(ctx, []string{sentTo})
	ns.sendNotification(ctx, payload, tokens)
}

func (ns NotificationService) sendTaskDenyWaitingRequestNotification(ctx context.Context, sentBy string, sentTo string, taskId string, taskTitle string, mid string) {
	sender, err := dbService.getUserById(ctx, sentBy)
	if err != nil {
		log.Println("Failed to fetch sender", sentBy, err)
		return
	}

	alertTitle := sender.FirstName + " " + sender.LastName

	payload := payload.NewPayload().AlertTitle(alertTitle).AlertSubtitle(taskTitle).AlertBody("Waiting Request Denied").ThreadID(taskId).Badge(1).Sound("default").MutableContent().Custom("mid", mid).Custom("uid", sentTo)
	tokens := ns.loadDeviceTokens(ctx, []string{sentTo})
	ns.sendNotification(ctx, payload, tokens)
}

func (ns NotificationService) sendChatTextMessageNotification(ctx context.Context, message string, sentBy string, chatId string, mid string) {
	sender, err := dbService.getUserById(ctx, sentBy)
	if err != nil {
		log.Println("Failed to fetch sender", sentBy, err)
		return
	}

	members, err := dbService.getChatGroupMembers(ctx, chatId)
	if err != nil {
		log.Println("Failed to fetch group members", sentBy, err)
		return
	}

	for _, m := range members {
		if m.MemberUserId == sentBy {
			continue
		}

		alertTitle := sender.FirstName + " " + sender.LastName
		payload := payload.NewPayload().AlertTitle(alertTitle).AlertBody(message).ThreadID(chatId).Badge(1).Sound("default").MutableContent().Custom("mid", mid).Custom("uid", m.MemberUserId)
		tokens := ns.loadDeviceTokens(ctx, []string{m.MemberUserId})
		ns.sendNotification(ctx, payload, tokens)
	}

}

func (ns NotificationService) sendGoodJobNotification(ctx context.Context, sentBy string, sentTo string, taskId string, taskTitle string, mid string, rtype MessageReactionType) {
	sender, err := dbService.getUserById(ctx, sentBy)
	if err != nil {
		log.Println("Failed to fetch sender", sentBy, err)
		return
	}

	alertTitle := sender.FirstName + " " + sender.LastName
	alertBody := "🎉 Good Job!"

	if rtype == MessageReactionThanks {
		alertBody = "👍 Thanks!"
	} else if rtype == MessageReactionWellDone {
		alertBody = "🌟 Well Done!"
	}
	payload := payload.NewPayload().AlertTitle(alertTitle).AlertSubtitle(taskTitle).AlertBody(alertBody).ThreadID(taskId).Badge(1).Sound("default").MutableContent().Custom("mid", mid).Custom("uid", sentTo)
	tokens := ns.loadDeviceTokens(ctx, []string{sentTo})
	ns.sendNotification(ctx, payload, tokens)

}

func (ns NotificationService) sendNotification(c context.Context, payload interface{}, deviceTokens []AddTokenModel) {
	for _, d := range deviceTokens {
		notification := &apns2.Notification{
			Topic:       "com.dulithadabare.klak",
			DeviceToken: d.Token,
			Payload:     payload,
		}

		res, err := ns.apnsClient.Push(notification)
		if err != nil {
			log.Println("There was an error sending notification", err)
			return
		}

		if res.Sent() {
			log.Println("Notification Sent:", res.ApnsID)
		} else {
			fmt.Printf("Failed to send notification: %v %v %v\n", res.StatusCode, res.ApnsID, res.Reason)
		}
	}
}
