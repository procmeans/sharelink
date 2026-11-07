// Package tiktok extracts watermark-free CDN URLs from TikTok video links.
//
// Unlike Douyin, TikTok's CDN rejects requests (403) that lack the
// tt_chain_token cookie issued with the video page and a tiktok.com Referer,
// so a Result carries the headers needed to fetch PlayURL.
package tiktok
