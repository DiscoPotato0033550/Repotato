package database

import (
	"context"
	"time"

	"github.com/jasonlvhit/gocron"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	messageCache = make(map[MessagePair]Message)
)

func init() {
	go func() {
		s := gocron.NewScheduler()
		s.Every(10).Hours().Do(func() {
			messageCache = make(map[MessagePair]Message)
		})
		<-s.Start()
	}()
}

type Message struct {
	GuildID   string       `bson:"guild_id" json:"guild_id"`
	Original  *MessagePair `bson:"original" json:"original"`
	Starboard *MessagePair `bson:"starboard" json:"starboard"`
	CreatedAt time.Time    `bson:"created_at" json:"created_at"`
}

type MessagePair struct {
	ChannelID string `bson:"channel_id" json:"channel_id"`
	MessageID string `bson:"message_id" json:"message_id"`
}

func NewMessage(original, starboard *MessagePair, guildID string) *Message {
	return &Message{
		GuildID:   guildID,
		Original:  original,
		Starboard: starboard,
		CreatedAt: time.Now(),
	}
}

func NewPair(channelID, messageID string) MessagePair {
	return MessagePair{
		ChannelID: channelID,
		MessageID: messageID,
	}
}

func InsertOneMessage(post *Message) error {
	collection := DB.Collection("messages")
	_, err := collection.InsertOne(context.Background(), post)
	if err != nil {
		return err
	}

	messageCache[NewPair(post.Original.ChannelID, post.Original.MessageID)] = *post
	return nil
}

func InsertManyMessages(posts []interface{}) error {
	collection := DB.Collection("messages")
	_, err := collection.InsertMany(context.Background(), posts)
	if err != nil {
		return err
	}

	for _, post := range posts {
		pair := *post.(Message).Original
		messageCache[NewPair(pair.ChannelID, pair.MessageID)] = post.(Message)
	}
	return nil
}

func DeleteMessage(pair *MessagePair) error {
	collection := DB.Collection("messages")
	_, err := collection.DeleteOne(context.Background(), bson.D{{"original.channel_id", pair.ChannelID}, {"original.message_id", pair.MessageID}})
	if err != nil {
		return err
	}

	delete(messageCache, *pair)
	return nil
}

func Repost(channelID, id string) (*Message, error) {
	m, ok := messageCache[NewPair(channelID, id)]

	if !ok {
		collection := DB.Collection("messages")
		res := collection.FindOne(context.Background(), bson.D{{"original.channel_id", channelID}, {"original.message_id", id}})
		if err := res.Err(); err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, nil
			}
			return nil, err
		}
		res.Decode(&m)
	}

	return &m, nil
}
