package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
		logError("missing_environment_variables", fmt.Errorf("required environment variables not set"), map[string]interface{}{
			"github_token_set": GITHUB_TOKEN != "",
			"github_user_set": GITHUB_USER != "",
			"discord_bot_token_set": DISCORD_BOT_TOKEN != "",
			"discord_user_id_set": DISCORD_USER_ID != "",
		})
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
	logInfo("application_started", map[string]interface{}{
		"github_user": GITHUB_USER,
		"timezone": getTimezone(),
	})

	// GitHub APIからコントリビューションデータを取得
	requestBody, err := json.Marshal(map[string]string{"query": QUERY})
	if err != nil {
		logError("failed_to_marshal_request", err, map[string]interface{}{
			"query_length": len(QUERY),
		})
		log.Fatal(err)
	}

	request, err := http.NewRequest("POST", URL, bytes.NewReader(requestBody))
	if err != nil {
		logError("failed_to_create_request", err, map[string]interface{}{
			"url": URL,
		})
		log.Fatal(err)
	}
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", GITHUB_TOKEN))
	request.Header.Set("Content-Type", "application/json")

	logInfo("github_api_request_start", map[string]interface{}{
		"url": URL,
		"user": GITHUB_USER,
	})

	client := new(http.Client)
	response, err := client.Do(request)
	if err != nil {
		logError("github_api_request_failed", err, map[string]interface{}{
			"url": URL,
			"user": GITHUB_USER,
		})
		log.Fatal(err)
	}
	defer response.Body.Close()

	// HTTPステータスコードのチェック
	if response.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(response.Body)
		logError("github_api_bad_status", fmt.Errorf("HTTP %d", response.StatusCode), map[string]interface{}{
			"status_code": response.StatusCode,
			"response_body": string(responseBody),
			"user": GITHUB_USER,
		})
		log.Fatalf("GitHub API returned status %d: %s", response.StatusCode, string(responseBody))
	}

	var githubContribution GithubContribution
	if err := json.NewDecoder(response.Body).Decode(&githubContribution); err != nil {
		logError("failed_to_decode_response", err, map[string]interface{}{
			"status_code": response.StatusCode,
		})
		log.Fatal(err)
	}

	logInfo("github_api_response_received", map[string]interface{}{
		"status_code": response.StatusCode,
		"weeks_count": len(githubContribution.Data.User.ContributionsCollection.ContributionCalendar.Weeks),
	})

	yesterdayContribution := 0
	todayContribution := 0

	// ホストのタイムゾーンを取得（環境変数TZまたはシステムデフォルト）
	var loc *time.Location
	if tz := os.Getenv("TZ"); tz != "" {
		var err error
		loc, err = time.LoadLocation(tz)
		if err != nil {
			logError("timezone_load_failed", err, map[string]interface{}{
				"requested_timezone": tz,
				"fallback_to": "Local",
			})
			loc = time.Local
		}
	} else {
		// 環境変数TZが設定されていない場合はローカルタイムゾーンを使用
		loc = time.Local
	}

	now := time.Now().In(loc)
	todayDate := now.Format("2006-01-02")
	yesterdayDate := now.AddDate(0, 0, -1).Format("2006-01-02")

	logInfo("timezone_info", map[string]interface{}{
		"timezone": loc.String(),
		"current_time": now.Format("2006-01-02 15:04:05 MST"),
		"today_date": todayDate,
		"yesterday_date": yesterdayDate,
	})

	for _, week := range githubContribution.Data.User.ContributionsCollection.ContributionCalendar.Weeks {
		for _, day := range week.ContributionDays {
			if day.Date == yesterdayDate {
				yesterdayContribution = day.ContributionCount
				logInfo("yesterday_contribution_found", map[string]interface{}{
					"date": day.Date,
					"contribution_count": day.ContributionCount,
				})
			}
			if day.Date == todayDate {
				todayContribution = day.ContributionCount
				logInfo("today_contribution_found", map[string]interface{}{
					"date": day.Date,
					"contribution_count": day.ContributionCount,
				})
			}
		}
	}

	// 連続日数の計算（今日が0なら昨日までを起点にカウント）
	// 日付 -> コントリビューション数 のマップを作成
	contributionsByDate := make(map[string]int)
	for _, week := range githubContribution.Data.User.ContributionsCollection.ContributionCalendar.Weeks {
		for _, day := range week.ContributionDays {
			contributionsByDate[day.Date] = day.ContributionCount
		}
	}

	// 起点日を決定（今日が0なら昨日、そうでなければ今日から）
	startDate := todayDate
	if contributionsByDate[todayDate] == 0 {
		startDate = yesterdayDate
	}

	// 連続日数を後ろ向きにカウント
	continueDays := 0
	current := startDate
	for {
		cnt, ok := contributionsByDate[current]
		if !ok || cnt == 0 {
			break
		}
		continueDays++
		t, err := time.Parse("2006-01-02", current)
		if err != nil {
			break
		}
		current = t.AddDate(0, 0, -1).Format("2006-01-02")
	}

	logInfo("contribution_summary", map[string]interface{}{
		"continue_days": continueDays,
		"today_contribution": todayContribution,
		"yesterday_contribution": yesterdayContribution,
		"start_date": startDate,
	})

	// DiscordにDMを送信
	message := fmt.Sprintf("昨日のコントリビューション数: %d", yesterdayContribution)
	if continueDays > 0 {
		message += fmt.Sprintf("\n連続日数: %d", continueDays)
	}
	if todayContribution == 0 {
		message += "\n今日のコントリビューションがまだです"
	}
	if todayContribution > 0 {
		message += fmt.Sprintf("\nおつかれさまでした: %d", todayContribution)
	}
	logInfo("sending_discord_message", map[string]interface{}{
		"message_length": len(message),
		"today_contribution": todayContribution,
		"yesterday_contribution": yesterdayContribution,
		"continue_days": continueDays,
	})

	err = sendDiscordDM(message)
	if err != nil {
		logError("discord_dm_send_failed", err, map[string]interface{}{
			"message_length": len(message),
			"discord_user_id": DISCORD_USER_ID,
		})
	} else {
		logInfo("discord_dm_sent_successfully", map[string]interface{}{
			"message_length": len(message),
			"discord_user_id": DISCORD_USER_ID,
		})
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
	logInfo("discord_session_init_start", nil)
	
	// 初回のみセッションを初期化
	if discordSession == nil {
		if err := initDiscord(); err != nil {
			logError("discord_session_init_failed", err, map[string]interface{}{
				"bot_token_length": len(DISCORD_BOT_TOKEN),
			})
			return err
		}
		defer discordSession.Close()
	}

	logInfo("discord_dm_channel_create_start", map[string]interface{}{
		"user_id": DISCORD_USER_ID,
	})

	// ユーザーとのDMチャンネルを作成
	channel, err := discordSession.UserChannelCreate(DISCORD_USER_ID)
	if err != nil {
		logError("discord_dm_channel_create_failed", err, map[string]interface{}{
			"user_id": DISCORD_USER_ID,
		})
		return fmt.Errorf("DMチャンネルの作成に失敗: %w", err)
	}

	logInfo("discord_message_send_start", map[string]interface{}{
		"channel_id": channel.ID,
		"message_length": len(content),
	})

	// メッセージを送信
	_, err = discordSession.ChannelMessageSend(channel.ID, content)
	if err != nil {
		logError("discord_message_send_failed", err, map[string]interface{}{
			"channel_id": channel.ID,
			"message_length": len(content),
		})
		return fmt.Errorf("メッセージの送信に失敗: %w", err)
	}

	return nil
}

// 構造化ログ用のヘルパー関数
func logInfo(event string, data map[string]interface{}) {
	logEntry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"level":     "INFO",
		"event":     event,
	}
	if data != nil {
		for k, v := range data {
			logEntry[k] = v
		}
	}
	jsonBytes, _ := json.Marshal(logEntry)
	fmt.Println(string(jsonBytes))
}

func logError(event string, err error, data map[string]interface{}) {
	logEntry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"level":     "ERROR",
		"event":     event,
		"error":     err.Error(),
	}
	if data != nil {
		for k, v := range data {
			logEntry[k] = v
		}
	}
	jsonBytes, _ := json.Marshal(logEntry)
	fmt.Println(string(jsonBytes))
}

func getTimezone() string {
	if tz := os.Getenv("TZ"); tz != "" {
		return tz
	}
	return "Local"
}
