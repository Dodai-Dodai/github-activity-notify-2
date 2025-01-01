package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	LINE_TOKEN        string
	GITHUB_TOKEN      string
	GITHUB_USER       string
	DISCORD_BOT_TOKEN string
	DISCORD_USER_ID   string
	URL               = "https://api.github.com/graphql"
	QUERY             string
)

type GithubContribution struct {
	Data struct {
		User struct {
			ContributionsCollection struct {
				ContributionCalendar struct {
					Weeks []struct {
						ContributionDays []struct {
							Color             string `json:"color"`
							ContributionCount int    `json:"contributionCount"`
							Date              string `json:"date"`
							Weekday           int    `json:"weekday"`
						} `json:"contributionDays"`
					} `json:"weeks"`
				} `json:"contributionCalendar"`
			} `json:"contributionsCollection"`
		} `json:"user"`
	} `json:"data"`
}

func init() {
	GITHUB_TOKEN = os.Getenv("GITHUB_TOKEN")
	GITHUB_USER = os.Getenv("GITHUB_USER")
	DISCORD_BOT_TOKEN = os.Getenv("DISCORD_BOT_TOKEN")
	DISCORD_USER_ID = os.Getenv("DISCORD_USER_ID")

	if GITHUB_TOKEN == "" || GITHUB_USER == "" || DISCORD_BOT_TOKEN == "" || DISCORD_USER_ID == "" {
		log.Fatal("必要な環境変数が設定されていません")
	}

	QUERY = fmt.Sprintf(`
    {
        user(login: "%s") {
            contributionsCollection {
                contributionCalendar {
                    weeks {
                        contributionDays {
                            color
                            contributionCount
                            date
                            weekday
                        }
                    }
                }
            }
        }
    }`, GITHUB_USER)
}

func main() {
	// GitHub APIからコントリビューションデータを取得
	requestBody, err := json.Marshal(map[string]string{"query": QUERY})
	if err != nil {
		log.Fatal(err)
	}
	request, err := http.NewRequest("POST", URL, bytes.NewReader(requestBody))
	if err != nil {
		log.Fatal(err)
	}
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", GITHUB_TOKEN))
	request.Header.Set("Content-Type", "application/json")

	client := new(http.Client)
	response, err := client.Do(request)
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	var githubContribution GithubContribution
	if err := json.NewDecoder(response.Body).Decode(&githubContribution); err != nil {
		log.Fatal(err)
	}

	yesterdayContribution := 0
	todayContribution := 0
	todayDate := time.Now().Format("2006-01-02")
	yesterdayDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	for _, week := range githubContribution.Data.User.ContributionsCollection.ContributionCalendar.Weeks {
		for _, day := range week.ContributionDays {
			if day.Date == yesterdayDate {
				fmt.Println("昨日:", day.Date, day.ContributionCount)
				yesterdayContribution = day.ContributionCount
			}
			if day.Date == todayDate {
				fmt.Println("今日:", day.Date, day.ContributionCount)
				todayContribution = day.ContributionCount
			}
		}
	}

	// 連続日数の計算（必要に応じて修正）
	continueDays := 0
	for _, week := range githubContribution.Data.User.ContributionsCollection.ContributionCalendar.Weeks {
		for _, day := range week.ContributionDays {
			if day.ContributionCount == 0 && day.Date != todayDate {
				continueDays = 0
			} else if day.Date == todayDate {
				continueDays++
			}
		}
	}
	fmt.Println("連続日数:", continueDays)
	fmt.Println("今日のコントリビューション数:", todayContribution)
	fmt.Println("昨日のコントリビューション数:", yesterdayContribution)

	// DiscordにDMを送信
	message := fmt.Sprintf("昨日のコントリビューション数: %d", yesterdayContribution)
	if continueDays > 0 {
		message += fmt.Sprintf("\n連続日数: %d", continueDays)
	}
	if todayContribution == 0 {
		message += "\n今日のコントリビューションがまだです"
	}
	err = sendDiscordDM(message)
	if err != nil {
		log.Printf("Discord DMの送信に失敗しました: %v", err)
	} else {
		log.Println("Discord DMを送信しました")
	}
}

// Discordセッションをグローバルに管理
var discordSession *discordgo.Session

func initDiscord() error {
	var err error
	discordSession, err = discordgo.New("Bot " + DISCORD_BOT_TOKEN)
	if err != nil {
		return fmt.Errorf("Discordセッションの作成に失敗: %w", err)
	}
	// Botを閉じる際にセッションを閉じる
	return discordSession.Open()
}

func sendDiscordDM(content string) error {
	// 初回のみセッションを初期化
	if discordSession == nil {
		if err := initDiscord(); err != nil {
			return err
		}
		defer discordSession.Close()
	}

	// ユーザーとのDMチャンネルを作成
	channel, err := discordSession.UserChannelCreate(DISCORD_USER_ID)
	if err != nil {
		return fmt.Errorf("DMチャンネルの作成に失敗: %w", err)
	}

	// メッセージを送信
	_, err = discordSession.ChannelMessageSend(channel.ID, content)
	if err != nil {
		return fmt.Errorf("メッセージの送信に失敗: %w", err)
	}

	return nil
}
