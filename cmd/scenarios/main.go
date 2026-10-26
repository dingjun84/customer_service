// Command scenarios 刷新 testdata 下的评测元数据（manifest.json +
// scenarios/test_scenarios.jsonl），不打开数据库、不需要 -reset。
//
//	go run ./cmd/scenarios -dir testdata
//
// 它会把手写的 IM 场景（testdata/im_export/scenarios_im.jsonl）合并进场景文件；
// 每条 IM 场景在合并前都会过一遍字段/枚举/工具白名单/PII 校验，不合格直接失败。
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/ecommerce-support/internal/seed"
)

func main() {
	dir := flag.String("dir", "testdata", "artifacts 根目录")
	flag.Parse()

	if err := seed.BuildArtifacts(*dir); err != nil {
		fmt.Fprintf(os.Stderr, "生成评测元数据失败: %v\n", err)
		os.Exit(1)
	}

	path := filepath.Join(*dir, "scenarios", "test_scenarios.jsonl")
	total, byCategory, bySplit := countScenarios(path)
	fmt.Printf("场景文件   %s（%d 条）\n", path, total)
	fmt.Printf("分区       dev %d / holdout %d\n", bySplit["dev"], bySplit["holdout"])
	fmt.Println("分类       " + joinCounts(byCategory))
}

func countScenarios(path string) (int, map[string]int, map[string]int) {
	byCategory := map[string]int{}
	bySplit := map[string]int{}
	f, err := os.Open(path)
	if err != nil {
		return 0, byCategory, bySplit
	}
	defer func() { _ = f.Close() }()

	total := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var row struct {
			Category string `json:"category"`
			Split    string `json:"split"`
		}
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		total++
		byCategory[row.Category]++
		bySplit[row.Split]++
	}
	return total, byCategory, bySplit
}

func joinCounts(m map[string]int) string {
	out := ""
	for k, v := range m {
		if out != "" {
			out += " / "
		}
		out += fmt.Sprintf("%s %d", k, v)
	}
	return out
}
