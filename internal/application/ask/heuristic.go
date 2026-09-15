package ask

import "strings"

const heuristicHighThreshold = 40

type heuristicLexicon struct {
	Language       string
	Greetings      []string
	SimpleTasks    []string
	ReasoningTerms []string
	ComplexTerms   []string
}

// Prompt language is independent of CLI locale, so every lexicon is scanned.
var heuristicLexicons = []heuristicLexicon{
	{
		Language:  "en",
		Greetings: []string{"hello", "hi", "hey", "ping", "who are you"},
		SimpleTasks: []string{
			"format", "json to yaml", "base64", "compress", "pretty print", "minify",
		},
		ReasoningTerms: []string{
			"why", "reason", "derive", "prove", "architecture", "design",
			"bug", "error", "panic", "exception", "deadlock", "memory leak",
			"optimize", "performance", "bottleneck", "refactor", "compare",
			"difference", "tradeoff", "root cause",
		},
		ComplexTerms: []string{
			"if ", "then ", "however", "because", "how to", "why",
		},
	},
	{
		Language:  "zh",
		Greetings: []string{"你好", "你是谁", "在吗"},
		SimpleTasks: []string{
			"格式化", "json转yaml", "base64", "压缩", "美化",
		},
		ReasoningTerms: []string{
			"为什么", "原因", "推导", "证明", "架构", "设计",
			"bug", "报错", "panic", "exception", "死锁", "内存泄漏",
			"优化", "性能", "瓶颈", "重构", "对比", "区别", "优缺点",
		},
		ComplexTerms: []string{
			"如果", "但是", "导致", "哪怕", "如何实现", "怎么解决",
		},
	},
	{
		Language:  "ja",
		Greetings: []string{"こんにちは", "おはよう", "誰ですか"},
		SimpleTasks: []string{
			"フォーマット", "整形", "変換", "圧縮",
		},
		ReasoningTerms: []string{
			"なぜ", "原因", "証明", "設計", "アーキテクチャ",
			"バグ", "エラー", "デッドロック", "メモリリーク",
			"最適化", "性能", "ボトルネック", "リファクタリング",
		},
		ComplexTerms: []string{"もし", "なぜ", "どうやって"},
	},
	{
		Language:  "ko",
		Greetings: []string{"안녕하세요", "누구세요"},
		SimpleTasks: []string{
			"포맷", "변환", "압축",
		},
		ReasoningTerms: []string{
			"왜", "원인", "증명", "설계", "아키텍처",
			"버그", "오류", "교착", "메모리 누수",
			"최적화", "성능", "병목", "리팩터링",
		},
		ComplexTerms: []string{"만약", "왜", "어떻게"},
	},
}

func shouldEnableThinking(prompt string, stdinCharacters int) bool {
	fullText := strings.TrimSpace(prompt)
	lowerText := strings.ToLower(fullText)
	normalized := strings.TrimSpace(strings.Trim(lowerText, ".!?？。！"))

	for _, lexicon := range heuristicLexicons {
		for _, greeting := range lexicon.Greetings {
			if normalized == greeting {
				return false
			}
		}
	}
	for _, lexicon := range heuristicLexicons {
		for _, task := range lexicon.SimpleTasks {
			if strings.Contains(lowerText, task) && len([]rune(fullText)) < 200 {
				return false
			}
		}
	}

	score := 0
	for _, lexicon := range heuristicLexicons {
		for _, term := range lexicon.ReasoningTerms {
			if strings.Contains(lowerText, term) {
				score += 40
			}
		}
	}
	if strings.Contains(fullText, "```") ||
		strings.Contains(lowerText, "goroutine") ||
		strings.Contains(lowerText, "traceback") ||
		strings.Contains(lowerText, "stack trace") {
		score += 30
	}
	for _, lexicon := range heuristicLexicons {
		found := false
		for _, term := range lexicon.ComplexTerms {
			if strings.Contains(lowerText, term) {
				score += 20
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if stdinCharacters > 500 {
		score += 20
	}
	if score >= heuristicHighThreshold {
		return true
	}
	return false
}
