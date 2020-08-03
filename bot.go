package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/VTGare/Eugen/database"
	"github.com/VTGare/Eugen/framework"
	"github.com/VTGare/Eugen/services"
	"github.com/VTGare/Eugen/utils"
	"github.com/bwmarrin/discordgo"
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

func emojiURL(emoji *discordgo.Emoji) string {
	url := fmt.Sprintf("https://cdn.discordapp.com/emojis/%v.", emoji.ID)
	if emoji.Animated {
		url += "gif"
	} else {
		url += "png"
	}

	return url
}

func findTenor(content string) string {
	tenor := ""
	if ind := strings.Index(content, "https://tenor.com/view/"); ind != -1 {
		if ws := strings.IndexAny(content[ind:], " \n"); ws == -1 {
			tenor = content[ind:]
		} else {
			tenor = content[ind : ws+ind]
		}

		log.Info(tenor)
	}

	return tenor
}

func reactCreated(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	guild, ok := database.GuildCache[r.GuildID]

	if ok && guild.Enabled && guild.StarboardChannel != "" && !guild.IsBanned(r.ChannelID) {
		repost, err := database.Repost(r.ChannelID, r.MessageID)
		handleError(s, r.ChannelID, err)

		m, err := s.ChannelMessage(r.ChannelID, r.MessageID)
		if err != nil {
			log.Errorln(err)
			return
		}

		ch, err := s.Channel(m.ChannelID)
		handleError(s, r.ChannelID, err)

		if m.Content == "" && len(m.Attachments) == 0 {
			return
		}

		for _, react := range m.Reactions {
			if strings.ToLower(react.Emoji.APIName()) == strings.Trim(guild.StarEmote, "<:>") && react.Count >= guild.MinimumStars {
				t, _ := m.Timestamp.Parse()
				messageURL := fmt.Sprintf("https://discord.com/channels/%v/%v/%v", r.GuildID, r.ChannelID, m.ID)

				msg := &discordgo.MessageSend{}

				footer := &discordgo.MessageEmbedFooter{}
				if guild.IsGuildEmoji() {
					footer.IconURL = emojiURL(react.Emoji)
					footer.Text = fmt.Sprintf("%v", react.Count)
				} else {
					footer.Text = fmt.Sprintf("%v %v", "⭐", react.Count)
				}

				embed := &discordgo.MessageEmbed{
					Author: &discordgo.MessageEmbedAuthor{
						Name:    fmt.Sprintf("%v in %v", m.Author.String(), ch.Name),
						URL:     messageURL,
						IconURL: m.Author.AvatarURL(""),
					},
					Color:       guild.EmbedColour,
					Description: fmt.Sprintf("%v\n\n[Click to jump to message!](%v)", m.Content, messageURL),
					Timestamp:   t.Format(time.RFC3339),
					Footer:      footer,
				}

				if len(m.Attachments) != 0 {
					if utils.ImageURLRegex.MatchString(m.Attachments[0].URL) {
						embed.Image = &discordgo.MessageEmbedImage{
							URL: m.Attachments[0].URL,
						}
					}
				} else if str := utils.ImageURLRegex.FindString(m.Content); str != "" {
					embed.Image = &discordgo.MessageEmbedImage{
						URL: str,
					}
					embed.Description = strings.Replace(embed.Description, str, "", 1)
				}

				if tenor := findTenor(embed.Description); tenor != "" && embed.Image == nil {
					res, err := services.Tenor(tenor)
					if err != nil {
						log.Warn(err)
					} else if len(res.Media) != 0 {
						embed.Description = strings.ReplaceAll(embed.Description, tenor, "")
						media := res.Media[0]
						embed.Image = &discordgo.MessageEmbedImage{
							URL: media.MediumGIF.URL,
						}
					}
				}

				if repost == nil {
					msg.Embed = embed
					log.Infof("Creating a new starboard. Guild: %v, channel: %v, message: %v", guild.Name, r.ChannelID, r.MessageID)
					starboard, err := s.ChannelMessageSendComplex(guild.StarboardChannel, msg)
					handleError(s, r.ChannelID, err)

					oPair := database.NewPair(m.ChannelID, m.ID)
					sPair := database.NewPair(starboard.ChannelID, starboard.ID)
					err = database.InsertOneMessage(database.NewMessage(&oPair, &sPair, r.GuildID))
					handleError(s, r.ChannelID, err)
				} else {
					log.Infof("Editing starboard (adding) %v in channel %v", repost.Starboard.MessageID, repost.Starboard.ChannelID)
					_, err := s.ChannelMessageEditEmbed(repost.Starboard.ChannelID, repost.Starboard.MessageID, embed)
					if err != nil {
						log.Warnln(err)
					}
				}

				return
			}
		}
	}
}

func reactRemoved(s *discordgo.Session, r *discordgo.MessageReactionRemove) {
	guild, ok := database.GuildCache[r.GuildID]

	if ok && guild.Enabled && guild.StarboardChannel != "" && !guild.IsBanned(r.ChannelID) {
		repost, err := database.Repost(r.ChannelID, r.MessageID)
		if err != nil {
			log.Warn(err)
		}

		if repost != nil {
			log.Infof("Editing starboard (subtracting) %v in channel %v", repost.Starboard.MessageID, repost.Starboard.ChannelID)
			m, err := s.ChannelMessage(r.ChannelID, r.MessageID)
			if err != nil {
				log.Warn(err)
			}
			starboard, err := s.ChannelMessage(repost.Starboard.ChannelID, repost.Starboard.MessageID)
			if err != nil {
				log.Warn(err)
			}

			if m == nil || starboard == nil {
				return
			}

			if len(m.Reactions) == 0 {
				oPair := database.NewPair(r.ChannelID, r.MessageID)
				err := database.DeleteMessage(&oPair)
				handleError(s, r.ChannelID, err)

				err = s.ChannelMessageDelete(starboard.ChannelID, starboard.ID)
				handleError(s, r.ChannelID, err)
			}

			for _, react := range m.Reactions {

				if strings.ToLower(react.Emoji.APIName()) == strings.Trim(guild.StarEmote, "<:>") {
					if react.Count <= guild.MinimumStars/2 {
						pair := database.NewPair(r.ChannelID, r.MessageID)
						err := database.DeleteMessage(&pair)
						if err != nil {
							log.Warn(err)
						}

						err = s.ChannelMessageDelete(starboard.ChannelID, starboard.ID)
						if err != nil {
							log.Warn(err)
						}
					} else {
						embed := starboard.Embeds[0]

						embed.Footer.Text = fmt.Sprintf("%v %v", "⭐", react.Count)
						_, err := s.ChannelMessageEditEmbed(starboard.ChannelID, starboard.ID, embed)
						if err != nil {
							log.Warn(err)
						}
					}
				}

				return
			}
		}
	}
}

func allReactsRemoved(s *discordgo.Session, r *discordgo.MessageReactionRemoveAll) {
	guild, ok := database.GuildCache[r.GuildID]

	if ok && guild.Enabled && guild.StarboardChannel != "" && !guild.IsBanned(r.ChannelID) {
		repost, err := database.Repost(r.ChannelID, r.MessageID)
		if err != nil {
			log.Warn(err)
		}

		if repost != nil {
			log.Infof("Removing starboard (all reactions removed) %v in channel %v", repost.Starboard.MessageID, repost.Starboard.ChannelID)
			err := s.ChannelMessageDelete(repost.Starboard.ChannelID, repost.Starboard.MessageID)
			if err != nil {
				log.Warnln(err)
			}

			pair := database.NewPair(r.ChannelID, r.MessageID)
			err = database.DeleteMessage(&pair)
			if err != nil {
				log.Warn(err)
			}
		}
	}
}

func messageDeleted(s *discordgo.Session, m *discordgo.MessageDelete) {
	guild, ok := database.GuildCache[m.GuildID]

	if ok && guild.Enabled && guild.StarboardChannel != "" && !guild.IsBanned(m.ChannelID) {
		repost, err := database.Repost(m.ChannelID, m.ID)
		if err != nil {
			log.Warn(err)
		}

		if repost != nil {
			log.Infof("Removing starboard (message deleted) %v in channel %v", repost.Starboard.MessageID, repost.Starboard.ChannelID)
			err := s.ChannelMessageDelete(repost.Starboard.ChannelID, repost.Starboard.MessageID)
			if err != nil {
				log.Warnln(err)
			}

			pair := database.NewPair(m.ChannelID, m.ID)
			err = database.DeleteMessage(&pair)
			if err != nil {
				log.Warn(err)
			}
		}
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

		database.GuildCache[g.ID] = *newGuild
		log.Infoln("Joined ", g.Name)
	}
}

func guildDeleted(s *discordgo.Session, g *discordgo.GuildDelete) {
	err := database.RemoveGuild(g.ID)
	if err != nil {
		log.Println(err)
	}

	delete(database.GuildCache, g.ID)
	log.Infoln("Kicked or banned from", g.Guild.Name, g.ID)
}
