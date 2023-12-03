package main

import (
	"context"
)

type DatabaseService struct {
	dynamoDbRespository *DynamoDbRepository
}

func (db DatabaseService) addUser(ctx context.Context, user AddUserModel) error {
	return db.dynamoDbRespository.addUser(ctx, user)
}

func (db DatabaseService) getUsers(ctx context.Context) ([]AddUserModel, error) {
	return db.dynamoDbRespository.getUsers(ctx)
}

func (db DatabaseService) getUserById(ctx context.Context, userId string) (AddUserModel, error) {
	return db.dynamoDbRespository.getUserById(ctx, userId)
}

func (db DatabaseService) addMessage(ctx context.Context, message ServerPush) error {
	return db.dynamoDbRespository.addMessage(ctx, message)
}

func (db DatabaseService) getMessages(ctx context.Context, userId string) ([]ServerPush, error) {
	return db.dynamoDbRespository.getMessages(ctx, userId)
}

func (db DatabaseService) addDeviceToken(ctx context.Context, token AddTokenModel) error {
	return db.dynamoDbRespository.addDeviceToken(ctx, token)
}

func (db DatabaseService) getDeviceTokens(ctx context.Context, userId string) ([]AddTokenModel, error) {
	return db.dynamoDbRespository.getDeviceTokens(ctx, userId)
}

func (db DatabaseService) addTask(ctx context.Context, task AddTaskModel) error {
	return db.dynamoDbRespository.addTask(ctx, task)
}

func (db DatabaseService) getTaskById(ctx context.Context, taskId string) (AddTaskModel, error) {
	return db.dynamoDbRespository.getTaskById(ctx, taskId)
}

func (db DatabaseService) addChatGroup(ctx context.Context, c AddChatGroupModel) error {
	return db.dynamoDbRespository.addChatGroup(ctx, c)
}

func (db DatabaseService) getChatGroupById(ctx context.Context, chatId string) (AddChatGroupModel, error) {
	return db.dynamoDbRespository.getChatGroupById(ctx, chatId)
}

func (db DatabaseService) addChatGroupMember(ctx context.Context, cgm AddChatGroupMemberModel) error {
	return db.dynamoDbRespository.addChatGroupMember(ctx, cgm)
}

func (db DatabaseService) getChatGroupMembers(ctx context.Context, chatId string) ([]AddChatGroupMemberModel, error) {
	return db.dynamoDbRespository.getChatGroupMembers(ctx, chatId)
}

func (db DatabaseService) getMessageById(ctx context.Context, mid, userId string) (ServerPush, error) {
	return db.dynamoDbRespository.getMessageById(ctx, mid, userId)
}

func (db DatabaseService) removeMessageById(ctx context.Context, mid, userId string) error {
	return db.dynamoDbRespository.deleteMessageById(ctx, mid, userId)
	// return errors.New("removeMessageById: function not implemented")
}
