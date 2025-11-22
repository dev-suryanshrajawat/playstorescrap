package scraper

import (
	"context"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
)

func FetchPlayStoreHTML(pkg string) (*goquery.Document, error) {

	url := "https://play.google.com/store/apps/details?id=" + pkg + "&hl=en&gl=US"

	// -----------------------------------------------
// CHROME OPTIONS (ADD PROXY HERE)
// -----------------------------------------------
opts := append(chromedp.DefaultExecAllocatorOptions[:],
    chromedp.ProxyServer("http://user:pass@ip:port"), // ✔ YOUR PROXY HERE
    chromedp.UserAgent("Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36"),
)

// Create Chrome allocator
allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
defer cancelAlloc()

// Create main browser context
ctx, cancel := chromedp.NewContext(allocCtx)
defer cancel()


	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var html string

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),

		// page title visible = base page loaded
		chromedp.WaitVisible(`h1 span`, chromedp.ByQuery),

		//WAIT UNTIL RATING BLOCK ACTUALLY LOADS
		chromedp.WaitVisible(`div[aria-label*="stars"]`, chromedp.ByQuery),

		// additional safety wait
		chromedp.Sleep(1*time.Millisecond),

		// capture full rendered DOM
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	)

	if err != nil {
		return nil, err
	}

	return goquery.NewDocumentFromReader(strings.NewReader(html))
}

