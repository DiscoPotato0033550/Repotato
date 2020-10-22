package database

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Guild struct {
	Prefix               string             `json:"prefix" bson:"prefix"`
	ID                   string             `json:"guild_id" bson:"guild_id"`
	Name                 string             `json:"name" bson:"name"`
	StarEmote            string             `json:"emote" bson:"emote"`
	EmbedColour          int64              `json:"color" bson:"color"`
	Enabled              bool               `json:"enabled" bson:"enabled"`
	StarboardChannel     string             `json:"starboard" bson:"starboard"`
	NSFWStarboardChannel string             `json:"nsfwstarboard" bson:"nsfwstarboard"`
	Selfstar             bool               `json:"selfstar" bson:"selfstar"`
	IgnoreBots           bool               `json:"ignorebots" bson:"ignorebots"`
	MinimumStars         int                `json:"stars" bson:"stars"`
	ChannelSettings      []*ChannelSettings `json:"channel_settings" bson:"channel_settings"`
	BlacklistedUsers     []string           `json:"blacklisted_users" bson:"blacklisted_users"`
	BannedChannels       []string           `json:"banned" bson:"banned"`
	CreatedAt            time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at" bson:"updated_at"`
}

type ChannelSettings struct {
	ID              string `json:"id" bson:"id"`
	StarRequirement int    `json:"star_requirement" bson:"star_requirement"`
}

func (g *Guild) StarsRequired(channelID string) int {
	for _, ch := range g.ChannelSettings {
		if ch.ID == channelID {
			return ch.StarRequirement
		}
	}
	return g.MinimumStars
}

func (g *Guild) ChannelSettingsToString() string {
	var sb strings.Builder
	if len(g.ChannelSettings) == 0 {
		return "none"
	}

	sb.WriteString(fmt.Sprintf("<#%v>: %v", g.ChannelSettings[0].ID, g.ChannelSettings[0].StarRequirement))
	if len(g.ChannelSettings) > 1 {
		for _, ch := range g.ChannelSettings[1:] {
			sb.WriteString(fmt.Sprintf(" | <#%v>: %v", ch.ID, ch.StarRequirement))
		}
	}

	return sb.String()
}

func (g *Guild) BlacklistedToString() string {
	var sb strings.Builder
	if len(g.BlacklistedUsers) == 0 {
		return "none"
	}

	sb.WriteString(fmt.Sprintf("<@%v>", g.BlacklistedUsers[0]))
	if len(g.ChannelSettings) > 1 {
		for _, user := range g.BlacklistedUsers[1:] {
			sb.WriteString(fmt.Sprintf("| <@%v>", user))
		}
	}

	return sb.String()
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
		Prefix:               "e!",
		ID:                   guildID,
		MinimumStars:         5,
		Name:                 guildName,
		StarEmote:            "⭐",
		Enabled:              true,
		Selfstar:             true,
		IgnoreBots:           false,
		EmbedColour:          4431601,
		StarboardChannel:     "",
		NSFWStarboardChannel: "",
		BlacklistedUsers:     make([]string, 0),
		ChannelSettings:      make([]*ChannelSettings, 0),
		BannedChannels:       make([]string, 0),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
}

//AllGuilds returns all guilds from a database.
func AllGuilds() []*Guild {
	collection := DB.Collection("guilds")
	cur, err := collection.Find(context.Background(), bson.M{})

	if err != nil {
		return []*Guild{}
	}

	guilds := make([]*Guild, 0)

	err = cur.All(context.Background(), &guilds)
	if err != nil {
		log.Println("Error decoding", err)
	}

	return guilds
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

func ReplaceGuild(guild *Guild) error {
	collection := DB.Collection("guilds")
	res := collection.FindOneAndReplace(context.Background(), bson.M{"guild_id": guild.ID}, guild)
	if err := res.Err(); err != nil {
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

	GuildCache[guildID] = guild
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

	GuildCache[guildID] = guild
	return nil
}

func BanUser(guildID, userID string) error {
	col := DB.Collection("guilds")

	res := col.FindOneAndUpdate(context.Background(), bson.M{
		"guild_id": guildID,
	}, bson.M{
		"$set": bson.M{
			"updated_at": time.Now(),
		},
		"$addToSet": bson.M{
			"blacklisted_users": userID,
		},
	}, options.FindOneAndUpdate().SetReturnDocument(options.After))

	guild := &Guild{}
	err := res.Decode(guild)
	if err != nil {
		return err
	}

	GuildCache[guildID] = guild
	return nil
}

func UnbanUser(guildID, userID string) error {
	col := DB.Collection("guilds")

	res := col.FindOneAndUpdate(context.Background(), bson.M{
		"guild_id": guildID,
	}, bson.M{
		"$set": bson.M{
			"updated_at": time.Now(),
		},
		"$pull": bson.M{
			"blacklisted_users": userID,
		},
	}, options.FindOneAndUpdate().SetReturnDocument(options.After))

	guild := &Guild{}
	err := res.Decode(guild)
	if err != nil {
		return err
	}

	GuildCache[guildID] = guild
	return nil
}

func SetStarRequirement(guildID, channelID string, stars int) error {
	col := DB.Collection("guilds")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cs := &ChannelSettings{channelID, stars}
	res := col.FindOneAndUpdate(ctx, bson.M{
		"guild_id":            guildID,
		"channel_settings.id": channelID,
	}, bson.M{
		"$set": bson.M{
			"updated_at":                          time.Now(),
			"channel_settings.$.star_requirement": stars,
		},
	}, options.FindOneAndUpdate().SetReturnDocument(options.After))

	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			res = col.FindOneAndUpdate(ctx, bson.M{
				"guild_id": guildID,
			}, bson.M{
				"$set": bson.M{
					"updated_at": time.Now(),
				},
				"$addToSet": bson.M{
					"channel_settings": cs,
				},
			}, options.FindOneAndUpdate().SetReturnDocument(options.After))
			if err := res.Err(); err != nil {
				if err != mongo.ErrNoDocuments {
					return err
				}
			}
		} else {
			return err
		}
	}

	guild := &Guild{}
	err := res.Decode(guild)
	if err != nil {
		return err
	}
	GuildCache[guildID] = guild
	return nil
}

func UnsetStarRequirement(guildID, channelID string) error {
	col := DB.Collection("guilds")

	res := col.FindOneAndUpdate(context.Background(), bson.M{
		"guild_id": guildID,
	}, bson.M{
		"$set": bson.M{
			"updated_at": time.Now(),
		},
		"$pull": bson.M{
			"channel_settings": bson.M{"id": channelID},
		},
	}, options.FindOneAndUpdate().SetReturnDocument(options.After))

	guild := &Guild{}
	err := res.Decode(guild)
	if err != nil {
		return err
	}

	GuildCache[guildID] = guild
	return nil
}
