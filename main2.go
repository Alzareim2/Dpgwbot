package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	Token string = "MTA5MzgxODA4ODYzMTMyMDYxNg.GK4b5Q.MkJ4vjswD8uetZFlsWPOnjggo-UdzI4APKAZLY"
)

type Giveaway struct {
	MessageID      string
	ChannelID      string
	GuildID        string
	RequiredRoleID string
	EndTime        time.Time
	Winners        int
}

var (
	giveaways []*Giveaway
)

func main() {
	dg, err := discordgo.New("Bot " + Token)
	if err != nil {
		fmt.Println("Erreur lors de la création du bot :", err)
		return
	}

	dg.AddHandler(ready)
	dg.AddHandler(guildCreate)
	dg.AddHandler(interactionCreate)
	dg.AddHandler(reactionAdd)

	err = dg.Open()
	if err != nil {
		fmt.Println("Erreur lors de l'ouverture de la connexion :", err)
		return
	}

	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			updateGiveawayEmbeds(dg)
		}
	}()

	fmt.Println("Bot en cours d'exécution. Appuyez sur CTRL-C pour quitter.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	ticker.Stop()
	dg.Close()
}

func ready(s *discordgo.Session, event *discordgo.Ready) {
	s.UpdateGameStatus(0, "Giveaways")
}

func guildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	_, err := s.ApplicationCommandCreate(s.State.User.ID, event.Guild.ID, &discordgo.ApplicationCommand{
		Name:        "dpgw",
		Description: "Create a giveaway",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "duration",
				Description: "Time of giveaway (ex: 1h, 30m, 45s)",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "description",
				Description: "Description of giveaway",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionRole,
				Name:        "required_role",
				Description: "Rôle required to join giveaway",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "winners",
				Description: "Number of winners",
				Required:    false,
			},
		},
	})
	if err != nil {
		fmt.Println("Erreur lors de la création de la commande :", err)
	}
}

func interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type == discordgo.InteractionApplicationCommand {
		cmdData := i.ApplicationCommandData()
		if cmdData.Name == "dpgw" {

			if i.GuildID == "" {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Message",
					},
				})
				return
			}

			perms, err := s.UserChannelPermissions(i.Member.User.ID, i.ChannelID)
			if err != nil || perms&discordgo.PermissionManageMessages == 0 {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "you don't have permissions",
					},
				})
				return
			}

			durationStr := cmdData.Options[0].StringValue()
			duration, err := time.ParseDuration(durationStr)
			if err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "invalid time",
					},
				})
				return
			}

			description := cmdData.Options[1].StringValue()
			requiredRoleID := ""
			requiredRoleName := ""
			for _, option := range cmdData.Options {
				if option.Name == "required_role" {
					role := option.RoleValue(s, "")
					requiredRoleID = role.ID
					requiredRoleName = role.Name
					break
				}
			}

			winners := 1
			if len(cmdData.Options) > 3 {
				winners = int(cmdData.Options[3].IntValue())
			}

			giveawayMessage := "🎉 **GIVEAWAY** 🎉\n\n%s\n\nReact with 🎁 to join!"
			if requiredRoleID != "" {
				giveawayMessage += "%s"
				giveawayMessage = fmt.Sprintf(giveawayMessage, description, requiredRoleName)
			} else {
				giveawayMessage = fmt.Sprintf(giveawayMessage, description)
			}

			embed := &discordgo.MessageEmbed{
				Description: fmt.Sprintf("**%s\n\n **  ✨ %d Winner(s) \n ✨Hosted by <@%s>\n\n***Requirements :***\n ✨ Roles required: <@&%s>\n\n React with 🎁 to enter the giveaway!", description, winners, i.Member.User.ID, requiredRoleID),
				Color:       0x00ff00, // green json color
				Footer: &discordgo.MessageEmbedFooter{
					Text: fmt.Sprintf("Remaining time : %s | %d Winner(s)", duration, winners),
				},
			}
			msg, _ := s.ChannelMessageSendEmbed(i.ChannelID, embed)
			s.MessageReactionAdd(i.ChannelID, msg.ID, "🎁")

			endTime := time.Now().Add(duration)
			giveaways = append(giveaways, &Giveaway{
				MessageID:      msg.ID,
				ChannelID:      i.ChannelID,
				GuildID:        i.GuildID,
				RequiredRoleID: requiredRoleID,
				EndTime:        endTime,
				Winners:        winners,
			})

			time.AfterFunc(duration, func() {
				winnersList := pickWinners(s, i.GuildID, i.ChannelID, msg.ID, requiredRoleID, winners)
				if len(winnersList) == 0 {
					s.ChannelMessageSend(i.ChannelID, "No winner for this giveaway.")
				} else {

					winnersMentions := make([]string, len(winnersList))
					for i, winner := range winnersList {
						winnersMentions[i] = fmt.Sprintf("<@%s>", winner)
					}
					embed.Description = fmt.Sprintf("🎉 **GIVEAWAY ENDED** 🎉\n\n%s\n\nWinner(s): %s", description, strings.Join(winnersMentions, ", "))
					s.ChannelMessageEditEmbed(i.ChannelID, msg.ID, embed)
					s.ChannelMessageSend(i.ChannelID, fmt.Sprintf("The winner(s) is/are %s! You won **%s**!", strings.Join(winnersMentions, ", "), description))
				}
			})

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
				Data: &discordgo.InteractionResponseData{
					Embeds: []*discordgo.MessageEmbed{embed},
				},
			})
		}
	}
}

func reactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	user, err := s.User(r.UserID)
	if err != nil {
		return
	}

	if r.UserID == s.State.User.ID || user.Bot {
		return
	}

	var giveaway *Giveaway
	for _, g := range giveaways {
		if g.MessageID == r.MessageID {
			giveaway = g
			break
		}
	}

	if giveaway == nil {
		return
	}

	if !hasRole(s, giveaway.GuildID, r.UserID, giveaway.RequiredRoleID) {

		err = s.MessageReactionRemove(r.ChannelID, r.MessageID, r.Emoji.APIName(), r.UserID)
		if err != nil {
			fmt.Println("Erreur lors de la suppression de la réaction :", err)
		}
	}
}

func hasRole(s *discordgo.Session, guildID, userID, roleID string) bool {

	if roleID == "" {
		return true
	}

	member, err := s.GuildMember(guildID, userID)
	if err != nil {
		return false
	}

	for _, r := range member.Roles {
		if r == roleID {
			return true
		}
	}

	return false
}

func pickWinners(s *discordgo.Session, guildID, channelID, messageID, requiredRoleID string, winners int) []string {
	users, err := s.MessageReactions(channelID, messageID, "🎁", 100, "", "")
	if err != nil {
		fmt.Println("Erreur lors de la récupération des réactions :", err)
		return []string{}
	}

	var validUserIDs []string
	for _, user := range users {
		if !user.Bot && (requiredRoleID == "" || hasRole(s, guildID, user.ID, requiredRoleID)) {
			validUserIDs = append(validUserIDs, user.ID)
		}
	}

	if len(validUserIDs) == 0 {
		return []string{}
	}

	if len(validUserIDs) <= winners {
		return validUserIDs
	}

	rand.Shuffle(len(validUserIDs), func(i, j int) { validUserIDs[i], validUserIDs[j] = validUserIDs[j], validUserIDs[i] })

	return validUserIDs[:winners]
}

func updateGiveawayEmbeds(s *discordgo.Session) {
	for _, giveaway := range giveaways {
		message, err := s.ChannelMessage(giveaway.ChannelID, giveaway.MessageID)
		if err != nil {
			continue
		}

		embed := message.Embeds[0]

		timeRemaining := time.Until(giveaway.EndTime)
		if timeRemaining < 0 {
			timeRemaining = 0
		}

		var timeRemainingText string
		if timeRemaining >= 1*time.Minute {
			remainingMinutes := int(timeRemaining.Minutes())
			timeRemainingText = fmt.Sprintf("%d minutes", remainingMinutes)
		} else {
			remainingSeconds := int(timeRemaining.Seconds())
			timeRemainingText = fmt.Sprintf("%d secondes", remainingSeconds)
		}

		embed.Footer = &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Time remaining : %s", timeRemainingText),
		}

		_, err = s.ChannelMessageEditEmbed(giveaway.ChannelID, giveaway.MessageID, embed)
		if err != nil {
			fmt.Println("Erreur lors de la mise à jour de l'embed :", err)
		}
	}
}
