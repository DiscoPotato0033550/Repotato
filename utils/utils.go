package utils

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/VTGare/Eugen/database"
	"github.com/bwmarrin/discordgo"
	log "github.com/sirupsen/logrus"
)

var (
	//ImageURLRegex is a regex for image URLs
	ImageURLRegex = regexp.MustCompile(`(?i)(http(s?):)([/|.|\w|\s|-])*\.(?:jpg|jpeg|gif|png|webp)`)
	//NumRegex is a terrible number regex. Gonna replace it with better code.
	NumRegex = regexp.MustCompile(`([0-9]+)`)
	//EmojiRegex matches some Unicode emojis, it's not perfect but better than nothing
	EmojiRegex = regexp.MustCompile(`(\x{00a9}|\x{00ae}|[\x{2000}-\x{3300}]|\x{d83c}[\x{d000}-\x{dfff}]|\x{d83d}[\x{d000}-\x{dfff}]|\x{d83e}[\x{d000}-\x{dfff}])`)
	//EmbedColor is a default Discord embed color
	EmbedColor = 0x439ef1
	//ErrNotEnoughArguments is a default error when not enough arguments were given
	ErrNotEnoughArguments = errors.New("not enough arguments")
	//ErrParsingArgument is a default error when provided arguments couldn't be parsed
	ErrParsingArgument = errors.New("error parsing arguments, please make sure all arguments are integers")
	//ErrNoPermission is a default error when user doesn't have enough permissions to execute a command
	ErrNoPermission = errors.New("you don't have permissions to execute this command")
)

//EmbedTimestamp returns currect time formatted to RFC3339 for Discord embeds
func EmbedTimestamp() string {
	return time.Now().Format(time.RFC3339)
}

func CreateDB(eventGuilds []*discordgo.Guild) error {
	allGuilds := database.AllGuilds()
	for _, guild := range *allGuilds {
		database.GuildCache[guild.ID] = guild
	}

	newGuilds := make([]interface{}, 0)
	for _, guild := range eventGuilds {
		if _, ok := database.GuildCache[guild.ID]; !ok {
			log.Infoln(guild.ID, "not found in database. Adding...")
			g := database.NewGuild(guild.Name, guild.ID)
			newGuilds = append(newGuilds, g)
			database.GuildCache[g.ID] = *g
		}
	}

	if len(newGuilds) > 0 {
		err := database.InsertManyGuilds(newGuilds)
		if err != nil {
			return err
		}
		log.Infoln("Successfully inserted all current guilds.")
	}

	log.Infoln(fmt.Sprintf("Connected to %v guilds", len(eventGuilds)))
	return nil
}

//MemberHasPermission checks if guild member has a permission to do something on a server.
func MemberHasPermission(s *discordgo.Session, guildID string, userID string, permission int) (bool, error) {
	member, err := s.State.Member(guildID, userID)
	if err != nil {
		if member, err = s.GuildMember(guildID, userID); err != nil {
			return false, err
		}
	}

	// Iterate through the role IDs stored in member.Roles
	// to check permissions
	for _, roleID := range member.Roles {
		role, err := s.State.Role(guildID, roleID)
		if err != nil {
			return false, err
		}
		if role.Permissions&permission != 0 {
			return true, nil
		}
	}

	return false, nil
}

//FormatBool returns human-readable representation of boolean
func FormatBool(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

//GetEmoji returns a guild emoji API name from Discord state
func GetEmoji(s *discordgo.Session, guildID, e string) (string, error) {
	emojis, err := s.GuildEmojis(guildID)
	if err != nil {
		return "", err
	}

	for _, emoji := range emojis {
		if str := fmt.Sprintf("<:%v>", strings.ToLower(emoji.APIName())); str == e {
			return str, nil
		}
	}

	return e, nil
}

func Map(vs []string, f func(string) string) []string {
	vsm := make([]string, len(vs))
	for i, v := range vs {
		vsm[i] = f(v)
	}
	return vsm
}
