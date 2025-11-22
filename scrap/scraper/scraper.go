package scraper

import (
	"context"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func FetchPlayStoreHTML(pkg string) (*goquery.Document, error) {

	url := "https://play.google.com/store/apps/details?id=" + pkg + "&hl=en&gl=US"

	// ---------------------------------------------------
	// 1) CHROME OPTIONS (Proxy + User-Agent)
	// ---------------------------------------------------
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ProxyServer("http://user:pass@ip:port"), // <-- YOUR PROXY HERE
		chromedp.UserAgent("Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36"),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// ---------------------------------------------------
	// 2) Timeout for whole scraping
	// ---------------------------------------------------
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// ---------------------------------------------------
	// 3) APPLY NETWORK HEADERS (VERY IMPORTANT)
	// ---------------------------------------------------
	err := chromedp.Run(ctx,
		network.Enable(),
		network.SetExtraHTTPHeaders(network.Headers{
			"User-Agent":      "Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
			"Accept-Language": "en-US,en;q=0.9",
		}),
	)
	if err != nil {
		return nil, err
	}

	// ---------------------------------------------------
	// 4) SCRAPE PAGE
	// ---------------------------------------------------
	var html string

	err = chromedp.Run(ctx,
		chromedp.Navigate(url),

		chromedp.WaitVisible(`h1 span`, chromedp.ByQuery),
		chromedp.WaitVisible(`div[aria-label*="stars"]`, chromedp.ByQuery),

		chromedp.Sleep(100*time.Millisecond),

		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	)
	if err != nil {
		return nil, err
	}

	return goquery.NewDocumentFromReader(strings.NewReader(html))
}
