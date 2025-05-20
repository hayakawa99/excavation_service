package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type EntityTopic struct {
	ID    uint
	Topic string
}

type TopicTrend struct {
	TopicID   uint
	Week      time.Time
	Score     float64
	TopTitle  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// isStorePageはURLが食べログの店舗ページであるかを判定します。
// 英語ページ、リストページ、まとめページ、レビューページなどは店舗ページとはみなしません。
func isStorePage(u *url.URL) bool {
	host := u.Host
	path := u.Path

	// Tabelog のみ対象
	if !strings.Contains(host, "tabelog.com") {
		log.Printf("DEBUG: isStorePage - Tabelog以外: %s", u.String())
		return false
	}

	// 英語ページは除外
	if strings.Contains(path, "/en/") {
		log.Printf("DEBUG: isStorePage - 英語ページ除外: %s", u.String())
		return false
	}

	// 除外すべきパスパターン（店舗情報ではないページ）
	excludedPatterns := []*regexp.Regexp{
		regexp.MustCompile(`/dtlrvwlst/`),    // レビューリストページ
		regexp.MustCompile(`/rvwr/`),         // レビュアーページ
		regexp.MustCompile(`/user/`),         // ユーザーページ
		regexp.MustCompile(`/member/`),       // 会員ページ
		regexp.MustCompile(`/review/`),       // 個別の口コミページ (例: /<store_id>/review/<review_id>/)
		regexp.MustCompile(`/diary/`),        // 日記ページ
		regexp.MustCompile(`/photo/`),        // 写真ページ (例: /<store_id>/photo/)
		regexp.MustCompile(`/dtlphotolst/`), // 写真リストページ
		regexp.MustCompile(`/dtlmenu/`),      // メニュー詳細ページ
		regexp.MustCompile(`/dtlmap/`),       // 詳細マップページ
		regexp.MustCompile(`/help/`),         // ヘルプページ
		regexp.MustCompile(`/terms/`),        // 利用規約
		regexp.MustCompile(`/sitemap/`),      // サイトマップ
		regexp.MustCompile(`/rstLst/`),       // 店舗リストページ (検索結果など)
		regexp.MustCompile(`/word/`),         // 用語解説など
		regexp.MustCompile(`/cond/`),         // 条件検索ページ
		regexp.MustCompile(`/catLst/`),       // カテゴリーリストページ
		regexp.MustCompile(`/aream/`),        // エリアマップページ
		regexp.MustCompile(`/wiki/`),         // Wikiページ
		regexp.MustCompile(`/favorite/`),     // お気に入りページ
		regexp.MustCompile(`/lunch/`),        // ランチ特集など、個別の店舗ページではないもの
		regexp.MustCompile(`/dinner/`),       // ディナー特集など
		regexp.MustCompile(`/party/`),        // パーティー特集など
		regexp.MustCompile(`/matome/`),       // まとめ記事
	}
	for _, p := range excludedPatterns {
		if p.MatchString(path) {
			log.Printf("DEBUG: isStorePage - 除外パターンに一致 (%s): %s", p.String(), u.String())
			return false
		}
	}

	// 店舗ページURLの典型的なパターンに合致するか正規表現でチェック
	// 例: https://tabelog.com/tokyo/A1311/A131105/13034566/
	// 例: https://tabelog.com/tokyo/A1311/A131105/13034566 (末尾スラッシュなし)
	//
	// 正規表現の説明:
	// tabelog\.com/           : ドメイン
	// [a-z]{2,8}/             : 都道府県コード (例: tokyo, osaka, fukuokaなど)
	// A\d{3,4}/               : 広域エリアコード (例: A1311)
	// A\d{3,6}/               : 詳細エリアコード (例: A131105)
	// (\d{8}|\d{10})/?$       : 8桁または10桁の店舗ID (これをキャプチャ)
	//                          末尾にスラッシュが有っても無くてもOK、URLの末尾であること
	//                           食べログの店舗IDは主に8桁か10桁が多いようです。
	storePageRegex := regexp.MustCompile(`tabelog\.com/[a-z]{2,8}/A\d{3,4}/A\d{3,6}/(\d{8}|\d{10})/?$`)
	if storePageRegex.MatchString(u.String()) {
		log.Printf("DEBUG: isStorePage - 店舗ページとして判定: %s", u.String())
		return true
	}

	log.Printf("DEBUG: isStorePage - 店舗ページではない (全てのチェックに失敗): %s", u.String())
	return false
}

// extractStoreNameはタイトル文字列から店舗名を抽出・整形します。
func extractStoreName(title string) string {
	originalTitle := title
	log.Printf("DEBUG: extractStoreName - Original: '%s'", originalTitle)

	// セパレータによる分割
	if strings.Contains(title, "|") {
		title = strings.Split(title, "|")[0]
	}
	if strings.Contains(title, "-") {
		title = strings.Split(title, "-")[0]
	}
	if strings.Contains(title, "/") {
		title = strings.Split(title, "/")[0]
	}

	// ユーザー名や特定のワードを除去
	title = regexp.MustCompile(`\s*(?:さん|氏|様|ちゃん|君)\s*`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`（名無し）`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`\s*\([^\)]*?\)`).ReplaceAllString(title, "") // 括弧と中身を除去
	title = regexp.MustCompile(`\s*\[[^\]]*?\]`).ReplaceAllString(title, "") // 角括弧と中身を除去
	title = regexp.MustCompile(`【[^】]*?】`).ReplaceAllString(title, "")     // 黒い角括弧と中身を除去
	title = regexp.MustCompile(`《[^》]*?》`).ReplaceAllString(title, "")     // 二重山括弧と中身を除去

	specificUsers := []string{
		"稲毛屋", "maro-j", "はらぺこ大将", "アユボワン！", "komedarian", "ものごころ", "nobuta-nobu", "ramen-king",
		"グルマン", "食いしん坊", "食べログ太郎", "レビュアー", "レビュアーマスター", "美食家", "グルメキング",
		"グルメ探偵", "食べログレビュアー", "アユボワン",
		"honnesan", "taniy", "dragonfly8810", "ropefish", "吉田R", "Wine, women an' song", "Shoebill",
		"クスクス", "トカトントンガラシ", "びしくれた", "おもひで定食", "たけ1025", "ヘル", "たらく 日暮里店",
		"ノブヒロ＠上野", "養和軒", "イドカヤ７９７", "シルクロード", "たけとんたんた", "カレーおじさん＼／", "ゆすけ",
		"玄海寿司 本店", "南幌",
	}
	for _, user := range specificUsers {
		title = strings.ReplaceAll(title, user, "")
	}

	// 一般的なレビュアー名やブログ、まとめ記事関連のワード
	title = regexp.MustCompile(`\b[a-zA-Z0-9_]{3,15}\b`).ReplaceAllString(title, "") // 半角英数字のユーザー名など
	title = regexp.MustCompile(`\b(?:イコット|クチコミ|食べログ)\b`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`人気店\d*選`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`おすすめランチ`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`の[お店|グルメ|ランチ|名店|人気店]`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`[\d]+選`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`【最新版】`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`～[^～]*～`).ReplaceAllString(title, "") // 全角チルダで囲まれた部分
	title = regexp.MustCompile(`食べログまとめ`).ReplaceAllString(title, "")

	// その他のクリーンアップ
	title = strings.ReplaceAll(title, "...", "")
	title = strings.ReplaceAll(title, "　", " ") // 全角スペースを半角に
	title = strings.ReplaceAll(title, "｜", "")
	title = strings.ReplaceAll(title, "ー", "") // 長音記号
	title = strings.ReplaceAll(title, "～", "")
	title = strings.ReplaceAll(title, "『", "")
	title = strings.ReplaceAll(title, "』", "")
	title = strings.ReplaceAll(title, "「", "")
	title = strings.ReplaceAll(title, "」", "")
	title = strings.ReplaceAll(title, `"`, "")

	// 複数スペースを1つにまとめ、前後のスペースを除去
	title = strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(title, " "))

	// 短すぎる、または日本語・英数字が全く含まれないタイトルは無効と判断
	// \p{Han}: 漢字, \p{Hiragana}: ひらがな, \p{Katakana}: カタカナ
	if len(title) < 2 || !regexp.MustCompile(`[a-zA-Z0-9\p{Han}\p{Hiragana}\p{Katakana}]`).MatchString(title) {
		log.Printf("WARNING: extractStoreNameが短すぎる、または有効な文字を含まない店舗名を生成 (元: '%s', 結果: '%s')", originalTitle, title)
		return ""
	}
	log.Printf("DEBUG: extractStoreName - Cleaned: '%s'", title)
	return title
}

// fetchStoreLinksFromMatomeは食べログのまとめ記事から店舗のリンクとタイトルを抽出します。
// 返り値は、キーが正規化されたURL、値が店舗名のmapです。
func fetchStoreLinksFromMatome(urlStr string, seenURLs map[string]bool) map[string]string {
	storeLinks := make(map[string]string)
	log.Printf("DEBUG: fetchStoreLinksFromMatome - URL取得中: %s", urlStr)
	resp, err := http.Get(urlStr)
	if err != nil {
		log.Printf("ERROR: まとめ記事取得失敗 %s: %v", urlStr, err)
		return storeLinks
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Printf("ERROR: まとめ記事解析失敗 %s: %v", urlStr, err)
		return storeLinks
	}

	baseURL, _ := url.Parse(urlStr)

	doc.Find(".shop-list__item a, .summary-shop__title a, a[href*='tabelog.com'][class*='js-spot-link']").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}
		parsed, err := url.Parse(href)
		if err != nil {
			log.Printf("DEBUG: fetchStoreLinksFromMatome - href解析失敗: %s, エラー: %v", href, err)
			return
		}
		resolved := baseURL.ResolveReference(parsed)

		// 食べログの店舗ページのみを対象とする (isStorePageで厳格に判定)
		if isStorePage(resolved) {
			// URLを正規化して重複チェック (末尾のスラッシュを削除)
			normalizedURL := resolved.String()
			if strings.HasSuffix(normalizedURL, "/") {
				normalizedURL = normalizedURL[:len(normalizedURL)-1]
			}

			if !seenURLs[normalizedURL] {
				text := strings.TrimSpace(s.Text())
				cleanText := extractStoreName(text)
				if cleanText != "" {
					storeLinks[normalizedURL] = cleanText // key: Normalized URL, value: Name
					seenURLs[normalizedURL] = true        // 既に処理したURLとして記録
					log.Printf("DEBUG: fetchStoreLinksFromMatome - 店舗発見: '%s' URL: %s", cleanText, resolved.String())
				} else {
					log.Printf("DEBUG: fetchStoreLinksFromMatome - 無効なタイトルをスキップ URL: %s (元のテキスト: '%s')", resolved.String(), text)
				}
			} else {
				log.Printf("DEBUG: fetchStoreLinksFromMatome - 重複URLのためスキップ: %s", normalizedURL)
			}
		}
	})
	return storeLinks
}

// fetchLinksFromListingPageは食べログのリストページから店舗のリンクとタイトルを抽出します。
// 返り値は、キーが正規化されたURL、値が店舗名のmapです。
func fetchLinksFromListingPage(urlStr string, seenURLs map[string]bool) map[string]string {
	storeLinks := make(map[string]string)
	log.Printf("DEBUG: fetchLinksFromListingPage - URL取得中: %s", urlStr)
	resp, err := http.Get(urlStr)
	if err != nil {
		log.Printf("ERROR: リストページ取得失敗 %s: %v", urlStr, err)
		return storeLinks
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Printf("ERROR: リストページ解析失敗 %s: %v", urlStr, err)
		return storeLinks
	}

	baseURL, _ := url.Parse(urlStr)

	doc.Find(".list-rst__title a, .list-rst__wrap a, a.list-rst__rst-name-target").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}
		parsed, err := url.Parse(href)
		if err != nil {
			log.Printf("DEBUG: fetchLinksFromListingPage - href解析失敗: %s, エラー: %v", href, err)
			return
		}
		resolved := baseURL.ResolveReference(parsed)

		// 食べログの店舗ページのみを対象とする (isStorePageで厳格に判定)
		if isStorePage(resolved) {
			// URLを正規化して重複チェック (末尾のスラッシュを削除)
			normalizedURL := resolved.String()
			if strings.HasSuffix(normalizedURL, "/") {
				normalizedURL = normalizedURL[:len(normalizedURL)-1]
			}

			if !seenURLs[normalizedURL] {
				text := strings.TrimSpace(s.Text())
				cleanText := extractStoreName(text)
				if cleanText != "" {
					storeLinks[normalizedURL] = cleanText // key: Normalized URL, value: Name
					seenURLs[normalizedURL] = true        // 既に処理したURLとして記録
					log.Printf("DEBUG: fetchLinksFromListingPage - 店舗発見: '%s' URL: %s", cleanText, resolved.String())
				} else {
					log.Printf("DEBUG: fetchLinksFromListingPage - 無効なタイトルをスキップ URL: %s (元のテキスト: '%s')", resolved.String(), text)
				}
			} else {
				log.Printf("DEBUG: fetchLinksFromListingPage - 重複URLのためスキップ: %s", normalizedURL)
			}
		}
	})
	return storeLinks
}

// StoreData は店舗の情報を保持する構造体です。
type StoreData struct {
	Name         string
	URL          string
	BudgetLunch  string
	BudgetDinner string
	Genre        string
	IsChain      bool
}

// collectStoreInfoは個別の店舗ページから店舗名、予算、ジャンルなどの情報を収集します。
// チェーン店や安価な店舗の除外はここで行うべきですが、現在の実装ではHTML解析による情報取得とフィルタリングは未実装です。
// この関数は現在、storeNameとURLを受け取ってStoreData構造体を返すダミー実装です。
func collectStoreInfo(storeName, urlStr string) *StoreData {
	log.Printf("DEBUG: collectStoreInfo - 収集開始: %s, %s", storeName, urlStr)

	// TODO: ここにrequests.Get(url)とgoqueryを使ったHTML解析を追加し、
	// 予算、ジャンル、チェーン店かの情報を取得するロジックを実装する必要があります。
	// 例:
	// resp, err := http.Get(urlStr)
	// if err != nil { /* エラーハンドリング */ return nil }
	// defer resp.Body.Close()
	// doc, err := goquery.NewDocumentFromReader(resp.Body)
	// // doc.Find(".rdheader-subinfo__item--genre").Text() などで情報を抽出

	// 仮のデータ構造
	storeData := &StoreData{
		Name:         storeName,
		URL:          urlStr,
		BudgetLunch:  "不明",  // 仮のデータ
		BudgetDinner: "不明", // 仮のデータ
		Genre:        "不明", // 仮のデータ
		IsChain:      false,  // 仮のデータ
	}

	// TODO: 以下にチェーン店や安価な店舗を除外するロジックを追加（HTML解析で値が取得できた場合）
	// if storeData.IsChain {
	//     log.Printf("INFO: collectStoreInfo - チェーン店のため除外: %s", storeData.Name)
	//     return nil
	// }
	// if storeData.BudgetLunch != "不明" { // 例: 昼予算が特定の金額未満の場合
	//     // budget_lunchを数値に変換する処理が必要
	//     // if parsedBudget < 1000 { ... }
	//     log.Printf("INFO: collectStoreInfo - 安価な店舗（昼予算）のため除外: %s", storeData.Name)
	//     return nil
	// }

	log.Printf("INFO: collectStoreInfo - 店舗情報を収集しました: %s", storeName)
	return storeData
}

// SearchBrave はBrave Search APIを使用して、指定されたクエリで検索し、関連する店舗のタイトルとURLを返します。
// main関数から呼び出せるように、関数名を大文字で開始しています。
func SearchBrave(query string) (string, string) {
	apiKey := os.Getenv("BRAVE_API_KEY")
	if apiKey == "" {
		log.Fatal("Fatal: BRAVE_API_KEY 環境変数が設定されていません")
	}

	// 検索クエリを調整: queryが既に「食べログ」を含んでいる場合、重複して追加しない
	adjustedQuery := query
	if !strings.Contains(strings.ToLower(query), "食べログ") {
		adjustedQuery = adjustedQuery + " 食べログ"
	}

	encodedQuery := url.QueryEscape(adjustedQuery)
	// count=20 を追加して、より多くの検索結果を取得
	apiURL := "https://api.brave.com/res/v1/web/search?q=" + encodedQuery + "&count=20"
	log.Printf("DEBUG: SearchBrave - Brave API URL: %s", apiURL)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Printf("ERROR: Brave HTTPリクエスト作成失敗: %v", err)
		return "", ""
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("ERROR: Brave検索失敗: %v", err)
		return "", ""
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("ERROR: Braveレスポンスボディ読み込み失敗: %v", err)
		return "", ""
	}
	log.Printf("DEBUG: Brave APIレスポンスボディ:\n%s", string(body)) // Brave APIレスポンスボディを詳細に出力

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		log.Printf("ERROR: Braveレスポンス解析失敗: %v", err)
		return "", ""
	}

	webResults, ok := data["web"].(map[string]interface{})
	if !ok {
		log.Printf("DEBUG: Braveレスポンスにwebセクションが存在しない")
		return "", ""
	}

	resultsRaw, ok := webResults["results"]
	if !ok {
		log.Printf("DEBUG: Braveレスポンスにresultsが存在しない")
		return "", ""
	}

	results, ok := resultsRaw.([]interface{})
	if !ok {
		log.Printf("DEBUG: Braveレスポンスのresultsが不正な形式")
		return "", ""
	}

	var combinedTitles string
	var uniqueTitles []string
	seenURLs := make(map[string]bool) // 処理済みURLを管理 (正規化されたURLをキーとする)
	collectedCount := 0
	maxTitles := 3 // 最終的にGPTに渡す店舗の最大数

	processingLimit := 50 // 例として、最初の50件の結果までチェック

	for i, item := range results {
		if collectedCount >= maxTitles || i >= processingLimit {
			break
		}

		r, ok := item.(map[string]interface{})
		if !ok {
			log.Printf("DEBUG: SearchBrave - Skipped non-map item: %v", item)
			continue
		}
		title, titleOk := r["title"].(string)
		urlStr, urlOk := r["url"].(string)

		if !titleOk || !urlOk {
			log.Printf("DEBUG: SearchBrave - Skipped item missing title or URL: %v", r)
			continue
		}
		log.Printf("DEBUG: SearchBrave - Processing result %d: URL='%s', Title='%s'", i, urlStr, title)

		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			log.Printf("DEBUG: SearchBrave - Failed to parse URL: %s, error: %v", urlStr, err)
			continue
		}

		// URLを正規化して重複チェックに使用
		normalizedURL := parsedURL.String()
		if strings.HasSuffix(normalizedURL, "/") {
			normalizedURL = normalizedURL[:len(normalizedURL)-1]
		}

		if seenURLs[normalizedURL] {
			log.Printf("DEBUG: SearchBrave - 重複URLのためスキップ: %s", normalizedURL)
			continue
		}

		// 食べログ以外のURLはスキップ
		if !strings.Contains(parsedURL.Host, "tabelog.com") {
			log.Printf("DEBUG: SearchBrave - Skipping non-tabelog URL: %s", urlStr)
			continue
		}

		// 食べログの「まとめ記事」の場合
		if strings.Contains(parsedURL.Path, "/matome/") {
			log.Printf("DEBUG: SearchBrave - Detected Tabelog Matome URL: %s", urlStr)
			// `seenURLs` を `WorkspaceStoreLinksFromMatome` に渡して、その中で重複を管理
			storeTitlesFromMatome := fetchStoreLinksFromMatome(urlStr, seenURLs)
			for _, storeTitle := range storeTitlesFromMatome {
				// ここではもう`seenURLs`で重複チェック済み
				if collectedCount < maxTitles {
					uniqueTitles = append(uniqueTitles, storeTitle)
					combinedTitles += storeTitle + "; "
					collectedCount++
					log.Printf("DEBUG: SearchBrave - Added store from Tabelog matome: '%s'", storeTitle)
				}
			}
		} else if strings.Contains(parsedURL.Path, "/rstLst/") { // 食べログのリストページ
			log.Printf("DEBUG: SearchBrave - Detected Tabelog Listing URL: %s", urlStr)
			// `seenURLs` を `WorkspaceLinksFromListingPage` に渡して、その中で重複を管理
			storesFromListing := fetchLinksFromListingPage(urlStr, seenURLs)
			for _, storeTitle := range storesFromListing {
				// ここではもう`seenURLs`で重複チェック済み
				if collectedCount < maxTitles {
					uniqueTitles = append(uniqueTitles, storeTitle)
					combinedTitles += storeTitle + "; "
					collectedCount++
					log.Printf("DEBUG: SearchBrave - Added store from Tabelog listing: '%s'", storeTitle)
				}
			}
		} else if isStorePage(parsedURL) { // 食べログの直接の店舗ページ
			log.Printf("DEBUG: SearchBrave - Detected valid Tabelog store URL: %s", urlStr)
			cleanTitle := extractStoreName(title)
			if cleanTitle != "" && collectedCount < maxTitles {
				uniqueTitles = append(uniqueTitles, cleanTitle)
				combinedTitles += cleanTitle + "; "
				seenURLs[normalizedURL] = true // 直接の店舗ページもseenURLsに追加
				collectedCount++
				log.Printf("DEBUG: SearchBrave - Added store directly from Tabelog store page: '%s'", cleanTitle)
			} else {
				if cleanTitle == "" {
					log.Printf("DEBUG: SearchBrave - Cleaned title is empty for URL: %s (Original title: '%s')", urlStr, title)
				}
			}
		} else {
			log.Printf("DEBUG: SearchBrave - Skipping non-target Tabelog URL (neither matome, listing, nor recognized store page): %s", urlStr)
		}
	}

	if len(uniqueTitles) == 0 {
		log.Printf("DEBUG: SearchBrave - No valid store titles collected.")
		return "", ""
	}

	topTitle := strings.Join(uniqueTitles, "; ")
	log.Printf("DEBUG: SearchBrave - Final combined for GPT: '%s', Top Title: '%s'", combinedTitles, topTitle)
	return combinedTitles, topTitle
}

func main() {
	// ロギング設定 (GORMのログレベルも含む)
	log.SetOutput(os.Stdout) // 標準出力にログを出す
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile) // タイムスタンプとファイル名を表示

	// GORMのデータベース接続
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatalf("Fatal: DATABASE_URL 環境変数が設定されていません。例: postgres://user:password@host:port/dbname")
	}
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("Fatal: DB接続失敗: %v", err)
	}

	// 自動マイグレーション (必要に応じてコメント解除)
	// db.AutoMigrate(&EntityTopic{}, &TopicTrend{})

	// 固定のトピック "西日暮里" を使用し、SearchBrave関数内で「食べログ」を付加します。
	topic := EntityTopic{
		ID:    1,
		Topic: "西日暮里",
	}

	// SearchBraveを呼び出し
	combinedTitles, topTitle := SearchBrave(topic.Topic) // 関数名を大文字で呼び出す
	if topTitle == "" || combinedTitles == "" {
		log.Printf("WARNING: Brave検索結果から有効な店舗名が見つかりませんでした: topic=%s", topic.Topic)
		return
	}

	// スコアリングと保存処理
	var existing TopicTrend
	if err := db.Where("topic_id = ? AND top_title = ?", topic.ID, topTitle).First(&existing).Error; err == nil {
		log.Printf("INFO: スキップ: 既に存在 title=%s", topTitle)
		return
	}

	score := analyzeWithGPT(combinedTitles)
	trend := TopicTrend{
		TopicID:   topic.ID,
		Week:      time.Now().Truncate(24 * time.Hour), // 日付のみ
		Score:     score,
		TopTitle:  topTitle,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&trend).Error; err != nil {
		log.Printf("ERROR: トレンド保存失敗: %v", err)
	} else {
		log.Printf("INFO: 保存完了: topic_id=%d title=\"%s\" score=%.2f", topic.ID, topTitle, score)
	}
}

// analyzeWithGPTは与えられた入力文字列をGPTに渡し、スコアを返します。
func analyzeWithGPT(input string) float64 {
	if strings.TrimSpace(input) == "" {
		log.Println("DEBUG: analyzeWithGPT - 入力が空です。スコア0を返します。")
		return 0
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("Fatal: OPENAI_API_KEY 環境変数が設定されていません")
	}

	payload := map[string]interface{}{
		"model": "gpt-3.5-turbo", // 使用するモデル
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "以下の店舗名のリストから、話題性を100点満点でスコアリングしてください。JSONで {\"score\": 数値 } の形で返してください。",
			},
			{
				"role":    "user",
				"content": input,
			},
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("ERROR: GPTリクエストペイロード作成失敗: %v", err)
		return 0
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Printf("ERROR: GPT HTTPリクエスト作成失敗: %v", err)
		return 0
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second} // タイムアウトを設定
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("ERROR: GPT呼び出し失敗: %v", err)
		return 0
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("ERROR: GPTレスポンスボディ読み込み失敗: %v", err)
		return 0
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("ERROR: GPT APIからエラーレスポンス: ステータスコード=%d, ボディ=%s", resp.StatusCode, string(body))
		return 0
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("ERROR: GPTレスポンス解析失敗: %v (ボディ: %s)", err, string(body))
		return 0
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		log.Printf("ERROR: GPTレスポンスにchoicesがないか空です: %v", string(body))
		return 0
	}

	message, ok := choices[0].(map[string]interface{})["message"].(map[string]interface{})
	if !ok {
		log.Printf("ERROR: GPTレスポンスのmessageが不正な形式です: %v", string(body))
		return 0
	}

	content, ok := message["content"].(string)
	if !ok {
		log.Printf("ERROR: GPTレスポンスのcontentが不正な形式です: %v", string(body))
		return 0
	}
	log.Printf("DEBUG: analyzeWithGPT - GPT Content: '%s'", content)

	// GPTのJSON出力を解析
	var parsed map[string]float64
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		log.Printf("ERROR: GPT出力のJSON変換失敗 (内容: %s): %v", content, err)
		return 0
	}

	score, ok := parsed["score"]
	if !ok {
		log.Printf("ERROR: GPTレスポンスJSONにscoreキーが存在しません: %v", content)
		return 0
	}
	log.Printf("DEBUG: analyzeWithGPT - スコア: %.2f", score)

	return score
}