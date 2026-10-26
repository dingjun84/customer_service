// Command imprep 把 duty-im-export 的原始 txt 转成候选池：
//
//	go run ./cmd/imprep -src <IM_ROOT> -out testdata/im_export
//
// 它是 P0/P1/P2 的唯一输入口。原始会话含手机号、门店电话与地址，所以这里
// 只写脱敏后的文本；跑完会打印丢弃数、脱敏命中数与角色占比，后者用于和
// 实施方案 §1.4 的 44/37/13/5 对照，判断解析有没有跑偏。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/ecommerce-support/internal/imexport"
)

func main() {
	fs := flag.NewFlagSet("imprep", flag.ExitOnError)
	src := fs.String("src", "../JDDC-Baseline-Seq2Seq/数据/duty-im-export", "duty-im-export 目录（IM_ROOT）")
	out := fs.String("out", filepath.Join("testdata", "im_export"), "产物目录")
	limit := fs.Int("limit", 0, "只处理前 N 个文件，0 表示全部")
	_ = fs.Parse(os.Args[1:])

	st, err := imexport.Run(imexport.Options{SourceDir: *src, OutDir: *out, Limit: *limit})
	if err != nil {
		fmt.Fprintf(os.Stderr, "预处理失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("产物      %s（%d 会话）\n", st.OutputFile, st.SessionsTotal)
	fmt.Printf("统计      %s\n", filepath.Join(*out, "stats.json"))
	fmt.Printf("文件      输入 %d / 解析 %d / 失败 %d / 空 %d\n",
		st.InputFiles, st.ParsedFiles, st.FailedFiles, st.EmptyFiles)
	fmt.Printf("丢弃      无人商家 %d / 无用户文字 %d（合计 %d）\n",
		st.DiscardedNoHumanMerchant, st.DiscardedNoUserText, st.SessionsDiscarded)
	fmt.Printf("消息      %d 条，顾客 %.1f%% / 商家 %.1f%% / AI %.1f%% / 系统 %.1f%%\n",
		st.MessagesTotal, st.Ratios["user"], st.Ratios["merchant"], st.Ratios["bot"], st.Ratios["system"])
	fmt.Printf("套话      自动回复 %d / AI 等待 %d / 媒体 %d / 订单卡 %d / 商品卡 %d\n",
		st.FlagAutoReply, st.FlagWaitBot, st.FlagMedia, st.FlagOrderCard, st.FlagProductCard)
	fmt.Printf("脱敏      手机 %d / 座机 %d / 长数字 %d / 联系号 %d / 地址 %d / 客服名 %d / URL %d / 邮箱 %d\n",
		st.RedactPhoneHits, st.RedactTelHits, st.RedactOrderIDHits, st.RedactContactHits,
		st.RedactAddressHits, st.RedactAgentNameHit, st.RedactMediaURLHits, st.RedactEmailHits)
	if n := len(st.Warnings); n > 0 {
		fmt.Printf("警告      %d 条（前 5 条）：\n", n)
		for i, w := range st.Warnings {
			if i >= 5 {
				break
			}
			fmt.Printf("  - %s\n", w)
		}
	}
}
