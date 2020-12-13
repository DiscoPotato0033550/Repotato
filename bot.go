package main

import (
	"fmt"
	"strings"

	"github.com/VTGare/Eugen/database"
	"github.com/VTGare/Eugen/framework"
	"github.com/VTGare/Eugen/utils"
	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

var (
	botMention      string
	defaultPrefixes = []string{"e!", "e.", "e "}
)

func onReady(s *discordgo.Session, e *discordgo.Ready) {
	botMention = "<@!" + e.User.ID + ">"
	log.Infoln(e.User.String(), "is ready.")
	err := utils.CreateDB(e.Guilds)
	if err != nil {
		log.Warnln("Error adding guilds: ", err)
	}
}

func trimPrefix(content, guildID string) string {
	guild, ok := database.GuildCache[guildID]
	var defaultPrefix bool
	if ok && guild.Prefix == "e!" {
		defaultPrefix = true
	} else if !ok {
		defaultPrefix = true
	} else {
		defaultPrefix = false
	}

	switch {
	case strings.HasPrefix(content, botMention):
		return strings.TrimPrefix(content, botMention)
	case defaultPrefix:
		for _, prefix := range defaultPrefixes {
			if strings.HasPrefix(content, prefix) {
				return strings.TrimPrefix(content, prefix)
			}
		}
	case !defaultPrefix && ok:
		return strings.TrimPrefix(content, guild.Prefix)
	default:
		return content
	}

	return content
}

func handleError(s *discordgo.Session, channelID string, err error) {
	if err != nil {
		log.Errorf("An error occured: %v", err)
		embed := &discordgo.MessageEmbed{
			Title: "Oops, something went wrong!",
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: "https://i.imgur.com/OZ1Al5h.png",
			},
			Description: fmt.Sprintf("***Error message:***\n%v\n", err),
			Color:       utils.EmbedColor,
			Timestamp:   utils.EmbedTimestamp(),
		}
		s.ChannelMessageSendEmbed(channelID, embed)
	}
}

func messageCreated(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot {
		return
	}

	isGuild := m.GuildID != ""
	m.Content = strings.ToLower(m.Content)

	where := func() string {
		if isGuild {
			g, _ := s.Guild(m.GuildID)
			return g.Name
		}
		return "DMs"
	}

	var content = trimPrefix(m.Content, m.GuildID)

	//if prefix wasn't trimmed
	if content == m.Content {
		return
	}

	fields := strings.Fields(content)
	if len(fields) == 0 {
		return
	}

	for _, group := range framework.CommandGroups {
		if command, ok := group.Commands[fields[0]]; ok {
			if !isGuild && command.GuildOnly {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%v command can't be executed in DMs or group chats", command.Name))
				return
			}
			go func() {
				log.Infof("Executing %v, requested by %v in %v", m.Content, m.Author.String(), where())
				err := command.Exec(s, m, fields[1:])
				handleError(s, m.ChannelID, err)
			}()

			break
		}
	}
}

func reactCreated(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	if guild, ok := database.GuildCache[r.GuildID]; ok {
		if !guild.Enabled || guild.StarboardChannel == "" {
			return
		}

		msg, err := s.ChannelMessage(r.ChannelID, r.MessageID)
		if err != nil {
			logrus.Warnf("reactCreated() -> s.ChannelMessage(): %v. Channel ID: %v, Message ID: %v", err, r.ChannelID, r.MessageID)
			return
		}

		if msg.Author != nil {
			if msg.Author.Bot && guild.IgnoreBots {
				return
			}

			for _, user := range guild.BlacklistedUsers {
				if msg.Author.ID == user {
					return
				}
			}
		}

		if guild.IsBanned(r.ChannelID) {
			return
		}

		if msg.Author.ID != s.State.User.ID {
			se, err := newStarboardEventAdd(s, r)
			if err != nil {
				log.Warnln("newStarboardEventAdd(): ", err)
				return
			}

			if se.React != nil {
				if se.React.Count < guild.StarsRequired(se.message.ChannelID) {
					return
				}

				p := database.NewPair(r.ChannelID, r.MessageID)
				starboardQueue.Push(p, se)
			}
		}
	}
}

func reactRemoved(s *discordgo.Session, r *discordgo.MessageReactionRemove) {
	guild, ok := database.GuildCache[r.GuildID]
	msg, err := s.ChannelMessage(r.ChannelID, r.MessageID)
	if err != nil {
		logrus.Warnf("reactRemoved() -> s.ChannelMessage(): %v. Channel ID: %v, Message ID: %v", err, r.ChannelID, r.MessageID)
		return
	}

	if ok && guild.Enabled && guild.StarboardChannel != "" && !guild.IsBanned(r.ChannelID) && msg.Author.ID != s.State.User.ID {
		se, err := newStarboardEventRemove(s, r)
		if err != nil {
			log.Warnln("newStarboardEventRemove():", err)
			return
		}
		p := database.NewPair(r.ChannelID, r.MessageID)
		starboardQueue.Push(p, se)
	}
}

func allReactsRemoved(s *discordgo.Session, r *discordgo.MessageReactionRemoveAll) {
	guild, ok := database.GuildCache[r.GuildID]
	msg, err := s.ChannelMessage(r.ChannelID, r.MessageID)
	if err != nil {
		logrus.Warnf("allReactsRemoved() -> s.ChannelMessage(): %v. Channel ID: %v, Message ID: %v", err, r.ChannelID, r.MessageID)
		return
	}

	if ok && guild.Enabled && guild.StarboardChannel != "" && !guild.IsBanned(r.ChannelID) && msg.Author.ID != s.State.User.ID {
		repost, err := database.Repost(r.ChannelID, r.MessageID)
		if err != nil {
			log.Warn(err)
		}

		if repost != nil {
			log.Infof("Removing starboard (all reactions removed) %v in channel %v", repost.Starboard.MessageID, repost.Starboard.ChannelID)
			err := s.ChannelMessageDelete(repost.Starboard.ChannelID, repost.Starboard.MessageID)
			if err != nil {
				log.Warnln("allReactsRemoved() -> s.ChannelMessageDelete(): ", err)
			}
		}
	}
}

func messageDeleted(s *discordgo.Session, m *discordgo.MessageDelete) {
	var (
		guild, ok = database.GuildCache[m.GuildID]
	)

	if ok && guild.Enabled && guild.StarboardChannel != "" && !guild.IsBanned(m.ChannelID) {
		se, err := newStarboardEventDeleted(s, m)
		if err != nil {
			log.Warnln("newStarboardEventDeleted(): ", err)
			return
		}
		p := database.NewPair(m.ChannelID, m.ID)
		starboardQueue.Push(p, se)
	}
}

func guildCreated(s *discordgo.Session, g *discordgo.GuildCreate) {
	if len(database.GuildCache) == 0 {
		return
	}

	if _, ok := database.GuildCache[g.ID]; !ok {
		newGuild := database.NewGuild(g.Name, g.ID)
		err := database.InsertOneGuild(newGuild)
		if err != nil {
			log.Println(err)
		}

		database.GuildCache[g.ID] = newGuild
		log.Infoln("Joined ", g.Name)
	}
}

func guildDeleted(s *discordgo.Session, g *discordgo.GuildDelete) {
	if !g.Unavailable {
		log.Infoln("Kicked/banned from a guild. ID: ", g.ID)
	} else {
		log.Infoln("Guild outage. ID: ", g.ID)
	}
}
