package database

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type Message struct {
	GuildID   string    `bson:"guild_id" json:"guild_id"`
	ChannelID string    `bson:"channel_id" json:"channel_id"`
	MessageID string    `bson:"message_id" json:"message_id"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

func NewMessage(guildID, channelID, messageID string) *Message {
	return &Message{
		GuildID:   guildID,
		ChannelID: channelID,
		MessageID: messageID,
		CreatedAt: time.Now(),
	}
}

func InsertOneMessage(post *Message) error {
	collection := DB.Collection("messages")
	_, err := collection.InsertOne(context.Background(), post)
	if err != nil {
		return err
	}

	return nil
}

func InsertManyMessages(posts []interface{}) error {
	collection := DB.Collection("messages")
	_, err := collection.InsertMany(context.Background(), posts)
	if err != nil {
		return err
	}

	return nil
}

func IsRepost(channelID, id string) (bool, error) {
	collection := DB.Collection("messages")
	res := collection.FindOne(context.Background(), bson.D{{"channel_id", channelID}, {"message_id", id}})
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			return false, nil
		} else {
			return false, err
		}
	}

	return true, nil
}
