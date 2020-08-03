package database

import (
	"context"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Guild struct {
	Prefix           string    `json:"prefix" bson:"prefix"`
	ID               string    `json:"guild_id" bson:"guild_id"`
	Name             string    `json:"name" bson:"name"`
	StarEmote        string    `json:"emote" bson:"emote"`
	EmbedColour      int       `json:"color" bson:"color"`
	Enabled          bool      `json:"enabled" bson:"enabled"`
	StarboardChannel string    `json:"starboard" bson:"starboard"`
	MinimumStars     int       `json:"stars" bson:"stars"`
	BannedChannels   []string  `json:"banned" bson:"banned"`
	CreatedAt        time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" bson:"updated_at"`
}

func (g *Guild) IsBanned(channelID string) bool {
	for _, id := range g.BannedChannels {
		if id == channelID {
			return true
		}
	}
	return false
}

func (g *Guild) IsGuildEmoji() bool {
	return strings.HasPrefix(g.StarEmote, "<:")
}

func NewGuild(guildName, guildID string) *Guild {
	return &Guild{
		Prefix:           "e!",
		ID:               guildID,
		MinimumStars:     5,
		Name:             guildName,
		StarEmote:        "⭐",
		Enabled:          true,
		EmbedColour:      4431601,
		StarboardChannel: "",
		BannedChannels:   make([]string, 0),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
}

//AllGuilds returns all guilds from a database.
func AllGuilds() *[]Guild {
	collection := DB.Collection("guilds")
	cur, err := collection.Find(context.Background(), bson.M{})

	if err != nil {
		return &[]Guild{}
	}

	guilds := make([]Guild, 0)
	cur.All(context.Background(), &guilds)

	if err != nil {
		log.Println("Error decoding", err)
	}

	return &guilds
}

//InsertOneGuild inserts one guild to a database
func InsertOneGuild(guild *Guild) error {
	collection := DB.Collection("guilds")
	_, err := collection.InsertOne(context.Background(), guild)
	if err != nil {
		return err
	}
	return nil
}

//InsertManyGuilds insert a bulk of guilds to a database
func InsertManyGuilds(guilds []interface{}) error {
	collection := DB.Collection("guilds")
	_, err := collection.InsertMany(context.Background(), guilds)
	if err != nil {
		return err
	}
	return nil
}

//RemoveGuild removes a guild from a database.
func RemoveGuild(guildID string) error {
	collection := DB.Collection("guilds")
	_, err := collection.DeleteOne(context.Background(), bson.M{"guild_id": guildID})
	if err != nil {
		return err
	}

	return nil
}

func BanChannel(guildID, channelID string) error {
	col := DB.Collection("guilds")

	res := col.FindOneAndUpdate(context.Background(), bson.M{
		"guild_id": guildID,
	}, bson.M{
		"$set": bson.M{
			"updated_at": time.Now(),
		},
		"$addToSet": bson.M{
			"banned": channelID,
		},
	}, options.FindOneAndUpdate().SetReturnDocument(options.After))

	guild := &Guild{}
	err := res.Decode(guild)
	if err != nil {
		return err
	}

	GuildCache[guildID] = *guild
	return nil
}

func UnbanChannel(guildID, channelID string) error {
	col := DB.Collection("guilds")

	res := col.FindOneAndUpdate(context.Background(), bson.M{
		"guild_id": guildID,
	}, bson.M{
		"$set": bson.M{
			"updated_at": time.Now(),
		},
		"$pull": bson.M{
			"banned": channelID,
		},
	}, options.FindOneAndUpdate().SetReturnDocument(options.After))

	guild := &Guild{}
	err := res.Decode(guild)
	if err != nil {
		return err
	}

	GuildCache[guildID] = *guild
	return nil
}
