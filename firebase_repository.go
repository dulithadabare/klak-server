package main

import (
	"context"
	"log"

	"firebase.google.com/go/v4/db"
)

type FirebaseRepository struct {
	client *db.Client
}

func (db FirebaseRepository) addUser(ctx context.Context, uid string, user AddUserModel) error {
	ref := db.client.NewRef(pathUsers)
	err := ref.Child(uid).Set(ctx, user)
	if err != nil {
		log.Fatalln("Error setting value:", err)
	}

	return nil
}

func (db FirebaseRepository) addMessage(ctx context.Context, m ServerPush) error {
	userMessagesRef := db.client.NewRef(pathUserMessages)

	err := userMessagesRef.Child(m.UserId).Child(m.Id).Set(ctx, m)
	if err != nil {
		return err
	}

	return nil
}
