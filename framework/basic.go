package framework

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/VTGare/Eugen/database"
	"github.com/VTGare/Eugen/utils"
	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	basicGroup := CommandGroup{
		Name:        "basic",
		Description: "General purpose commands.",
		NSFW:        false,
		Commands:    make(map[string]Command),
		IsVisible:   true,
	}

	pingCommand := newCommand("ping", "Checks if Boe Tea is online and sends response time back.")
	pingCommand.setExec(ping)
	helpCommand := newCommand("help", "Sends this message. Use ``bt!help <group name> <command name>`` for more info about specific commands. ``bt!help <group>`` to list commands in a group.")
	helpCommand.setExec(help)
	setCommand := newCommand("set", "Show server's settings or change them.").setExec(set).setAliases("settings", "config", "cfg").setHelp(&HelpSettings{
		IsVisible: true,
		ExtendedHelp: []*discordgo.MessageEmbedField{
			{
				Name:  "Usage",
				Value: "e!set ``<setting>`` ``<new setting>``",
			},
			{
				Name:  "prefix",
				Value: "Changes bot's prefix. Maximum ***5 characters***. If last character is a letter whitespace is assumed (takes one character).",
			},
			{
				Name:  "enabled",
				Value: "Starboard functionality switch, accepts ***f or false (case-insensitive)*** to disable and ***t or true*** to enable.",
			},
			{
				Name:  "channel",
				Value: "Starboard channel. Required for starboard to work. Accepts channel ID or channel mention.",
			},
			{
				Name:  "emote",
				Value: "Starboard reaction emote.",
			},
			{
				Name:  "stars",
				Value: "Stars required to repost a message to starboard channel.",
			},
		},
	}).setGuildOnly(true)

	banCommand := newCommand("ban", "Bans a channel").setExec(ban).setGuildOnly(true)
	unbanCommand := newCommand("unban", "Unbans a channel").setExec(unban).setGuildOnly(true)
	reqCommand := newCommand("req", "Sets per channel star requirement").setExec(req).setGuildOnly(true).setAliases("requirement", "channelstars", "channelset")
	reqCommand.Help.ExtendedHelp = []*discordgo.MessageEmbedField{
		{
			Name:  "Usage",
			Value: "e!req <channel id or mention> <star requirement>",
		},
		{
			Name:  "Channel ID or mention",
			Value: "Required. It must be a channel on this server!",
		},
		{
			Name:  "Star requirement",
			Value: "Required. It must be an integer greater than or equal to 1 or ``default`` to remove a custom star requirement.",
		},
	}

	inviteCmd := newCommand("invite", "Sends an invite link").setExec(invite)

	basicGroup.addCommand(pingCommand)
	basicGroup.addCommand(helpCommand)
	basicGroup.addCommand(setCommand)
	basicGroup.addCommand(banCommand)
	basicGroup.addCommand(unbanCommand)
	basicGroup.addCommand(reqCommand)
	basicGroup.addCommand(inviteCmd)
	CommandGroups["basic"] = basicGroup
}

func ping(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	_, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(":ping_pong: Pong! Latency: ***%v***", s.HeartbeatLatency().Round(1*time.Millisecond)))
	if err != nil {
		return err
	}
	return nil
}

func help(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	embed := &discordgo.MessageEmbed{
		Description: "Use ``e!help <group name> <command name>`` for extended help on specific commands.",
		Color:       utils.EmbedColor,
		Timestamp:   utils.EmbedTimestamp(),
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: "https://i.imgur.com/OZ1Al5h.png",
		},
	}

	switch len(args) {
	case 0:
		embed.Title = "Help"
		for _, group := range CommandGroups {
			if group.IsVisible {
				field := &discordgo.MessageEmbedField{
					Name:  group.Name,
					Value: group.Description,
				}
				embed.Fields = append(embed.Fields, field)
			}
		}
	case 1:
		if group, ok := CommandGroups[args[0]]; ok {
			embed.Title = fmt.Sprintf("%v group command list", args[0])

			used := map[string]bool{}
			for _, command := range group.Commands {
				_, ok := used[command.Name]
				if command.Help.IsVisible && !ok {
					field := &discordgo.MessageEmbedField{
						Name:  command.Name,
						Value: command.createHelp(),
					}
					used[command.Name] = true
					embed.Fields = append(embed.Fields, field)
				}
			}
		} else {
			return fmt.Errorf("unknown group %v", args[0])
		}
	case 2:
		if group, ok := CommandGroups[args[0]]; ok {
			if command, ok := group.Commands[args[1]]; ok {
				if command.Help.IsVisible && command.Help.ExtendedHelp != nil {
					embed.Title = fmt.Sprintf("%v command extended help", command.Name)
					embed.Fields = command.Help.ExtendedHelp
				} else {
					return fmt.Errorf("command %v is invisible or doesn't have extended help", args[0])
				}
			} else {
				return fmt.Errorf("unknown command %v", args[1])
			}
		} else {
			return fmt.Errorf("unknown group %v", args[0])
		}
	default:
		return errors.New("incorrect command usage. Example: bt!help <group> <command name>")
	}

	s.ChannelMessageSendEmbed(m.ChannelID, embed)
	return nil
}

func ban(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	if len(args) == 0 {
		return utils.ErrNotEnoughArguments
	}

	guild := database.GuildCache[m.GuildID]
	for _, arg := range args {
		if strings.HasPrefix(arg, "<#") {
			arg = strings.Trim(arg, "<#>")
		}
		ch, err := s.Channel(arg)
		if err != nil {
			return err
		}
		if ch.GuildID == m.GuildID {
			if !guild.IsBanned(ch.ID) {
				err := database.BanChannel(ch.GuildID, ch.ID)
				if err != nil {
					return err
				}
			}
		}
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully banned following channels: %v", args))
	return nil
}

func unban(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	if len(args) == 0 {
		return utils.ErrNotEnoughArguments
	}

	guild := database.GuildCache[m.GuildID]
	for _, arg := range args {
		if strings.HasPrefix(arg, "<#") {
			arg = strings.Trim(arg, "<#>")
		}
		ch, err := s.Channel(arg)
		if err != nil {
			return err
		}
		if ch.GuildID == m.GuildID {
			if guild.IsBanned(ch.ID) {
				err := database.UnbanChannel(ch.GuildID, ch.ID)
				if err != nil {
					return err
				}
			}
		}
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully unbanned following channels: %v", args))
	return nil
}

func req(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	if len(args) < 2 {
		return utils.ErrNotEnoughArguments
	}
	channelID := ""

	channels, err := s.GuildChannels(m.GuildID)
	if err != nil {
		return err
	}

	if strings.HasPrefix(args[0], "<#") {
		channelID = strings.Trim(args[0], "<#>")
	}

	if !utils.IsValidChannel(channelID, channels) {
		return errors.New("you're trying to modify a foreign channel")
	}

	if args[1] == "default" {
		g := database.GuildCache[m.GuildID]

		f := false
		for _, ch := range g.ChannelSettings {
			if ch.ID == channelID {
				f = true
			}
		}

		if f {
			err = database.UnsetStarRequirement(m.GuildID, channelID)
			if err != nil {
				return err
			}
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully reset <#%v> settings to defaults", channelID))
		} else {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Can't reset <#%v> to defaults, channel doesn't have star requirements set.", channelID))
		}
	} else {
		stars, err := strconv.Atoi(args[1])
		if err != nil {
			return utils.ErrParsingArgument
		}

		if stars < 1 {
			return fmt.Errorf("Star requirement should be >= 1, provided star requirement is %v", stars)
		}

		err = database.SetStarRequirement(m.GuildID, channelID, stars)
		if err != nil {
			return fmt.Errorf("database error\n%v", err)
		}
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully set <#%v> star requirement to %v", channelID, stars))
	}
	return nil
}

func set(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	switch len(args) {
	case 0:
		showGuildSettings(s, m)
	case 2:
		isAdmin, err := utils.MemberHasPermission(s, m.GuildID, m.Author.ID, discordgo.PermissionAdministrator)
		if err != nil {
			return err
		}
		if !isAdmin {
			return utils.ErrNoPermission
		}

		setting := args[0]
		newSetting := strings.ToLower(args[1])

		var passedSetting interface{}
		switch setting {
		case "enabled":
			passedSetting, err = strconv.ParseBool(newSetting)
		case "color":
			passedSetting, err = strconv.Atoi(newSetting)
			if passedSetting.(int) > 16777215 || passedSetting.(int) < 0 {
				return errors.New("non-existing decimal color, it should be in range from 0 to 16777215")
			}
		case "prefix":
			if unicode.IsLetter(rune(newSetting[len(newSetting)-1])) {
				passedSetting = newSetting + " "
			} else {
				passedSetting = newSetting
			}

			if len(passedSetting.(string)) > 5 {
				return errors.New("new prefix is too long")
			}
		case "stars":
			passedSetting, err = strconv.Atoi(newSetting)
		case "emote":
			emoji, err := utils.GetEmoji(s, m.GuildID, newSetting)
			if err != nil {
				return errors.New("argument's either global emoji or not one at all")
			}
			passedSetting = emoji
		case "starboard":
			if strings.HasPrefix(newSetting, "<#") {
				newSetting = strings.TrimSuffix(strings.TrimPrefix(newSetting, "<#"), ">")
			}
			ch, err := s.Channel(newSetting)
			if err != nil {
				return err
			}
			if ch.GuildID != m.GuildID {
				return errors.New("can't assign starboard to a channel from a foreign server")
			}

			passedSetting = newSetting
		default:
			return errors.New("unknown setting " + setting)
		}

		if err != nil {
			return err
		}

		err = changeSetting(m.GuildID, setting, passedSetting)
		if err != nil {
			return err
		}
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully changed ``%v`` to ``%v``", setting, newSetting))
	default:
		return errors.New("incorrect command usage. Please use e!help set command for more information")
	}

	return nil
}

func showGuildSettings(s *discordgo.Session, m *discordgo.MessageCreate) {
	settings := database.GuildCache[m.GuildID]
	guild, _ := s.Guild(settings.ID)

	banned := strings.Join(utils.Map(settings.BannedChannels, func(s string) string {
		return fmt.Sprintf("<#%v>", s)
	}), " | ")
	if banned == "" {
		banned = "none"
	}

	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Title:       "Current settings",
		Description: guild.Name,
		Color:       settings.EmbedColour,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "Starboard",
				Value: utils.FormatBool(settings.Enabled),
			},
			{
				Name:  "Settings",
				Value: fmt.Sprintf("Channel: <#%v> | Emoji: %v | Min stars: %v | Prefix: %v", settings.StarboardChannel, settings.StarEmote, settings.MinimumStars, settings.Prefix),
			},
			{
				Name:  "Unique star requirements",
				Value: settings.ChannelSettingsToString(),
			},
			{
				Name:  "Banned channels",
				Value: banned,
			},
		},
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: guild.IconURL(),
		},
		Timestamp: utils.EmbedTimestamp(),
	})
}

func changeSetting(guildID, setting string, newSetting interface{}) error {
	col := database.DB.Collection("guilds")

	res := col.FindOneAndUpdate(context.Background(), bson.M{
		"guild_id": guildID,
	}, bson.M{
		"$set": bson.M{
			setting:      newSetting,
			"updated_at": time.Now(),
		},
	}, options.FindOneAndUpdate().SetReturnDocument(options.After))

	guild := &database.Guild{}
	err := res.Decode(guild)
	if err != nil {
		return err
	}

	database.GuildCache[guildID] = guild
	return nil
}

func invite(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	embed := &discordgo.MessageEmbed{
		Title:       "Thanks for spreading the word!",
		Description: "Eugen loves you :)\nhttps://discord.com/api/oauth2/authorize?client_id=738399095378673786&permissions=379968&scope=bot",
		Image:       &discordgo.MessageEmbedImage{URL: s.State.User.AvatarURL("")},
	}

	s.ChannelMessageSendEmbed(m.ChannelID, embed)
	return nil
}
