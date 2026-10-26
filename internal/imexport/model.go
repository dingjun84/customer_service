// Package imexport 把美团门店 IM 导出（duty-im-export）转成 customer_service
// 能消费的候选池，供评测集、口吻 few-shot 与意图词表使用。
//
// 这个包的输出是唯一允许进入下游的通道：下游只读 candidates.jsonl，不再自己
// 扫原始 txt。原始对话含有手机号、门店电话与地址，因此所有落盘文本都必须是
// 脱敏后的 text_redacted，原文不写盘、不进库、不进日志。
package imexport

// Turn 是一条脱敏后的消息。
type Turn struct {
	Idx          int      `json:"idx"`
	TS           string   `json:"ts"`
	Role         string   `json:"role"`     // user | merchant | bot | system
	MsgType      string   `json:"msg_type"` // 文字 / 图片 / 商品卡 / 5 / 语音 ...
	Flags        []string `json:"flags"`
	TextRedacted string   `json:"text_redacted"`
}

// SessionFlags 是会话级的信号位，用来筛转人工与投诉样本。
type SessionFlags struct {
	HasComplaintCue bool `json:"has_complaint_cue"`
	HasHumanRequest bool `json:"has_human_request"`
	HasMedia        bool `json:"has_media"`
}

// Candidate 是 candidates.jsonl 的一行。字段名与实施方案 §2.6 一致。
type Candidate struct {
	SourceFile       string       `json:"source_file"`
	StoreID          string       `json:"store_id"`
	VendorID         string       `json:"vendor_id"`
	ESID             string       `json:"es_id"`
	GroupID          string       `json:"group_id"`
	TaskIDs          []string     `json:"task_ids"`
	MessageCount     int          `json:"message_count"`
	UserTextCount    int          `json:"user_text_count"`
	MerchantHumanCnt int          `json:"merchant_human_count"`
	DiscardAsGold    bool         `json:"discard_as_gold"`
	DiscardReason    string       `json:"discard_reason,omitempty"`
	Turns            []Turn       `json:"turns"`
	FlagsSession     SessionFlags `json:"flags_session"`
	CategoryGuess    *string      `json:"category_guess"`
	Notes            string       `json:"notes,omitempty"`
}

// Stats 是 im_export/stats.json 的结构。丢弃数与脱敏命中数按类型分开记，
// 回报模板要求逐项列出，所以这里显式命名而不是塞进 map。
type Stats struct {
	SourceRoot  string `json:"source_root"`
	OutputFile  string `json:"output_file"`
	GeneratedAt string `json:"generated_at"`

	InputFiles  int `json:"input_files"`
	ParsedFiles int `json:"parsed_files"`
	FailedFiles int `json:"failed_files"`
	EmptyFiles  int `json:"empty_files"`

	SessionsTotal     int `json:"sessions_total"`
	SessionsKept      int `json:"sessions_kept"`
	SessionsDiscarded int `json:"sessions_discarded"`

	DiscardedNoHumanMerchant int `json:"discarded_no_human_merchant"`
	DiscardedNoUserText      int `json:"discarded_no_user_text"`

	MessagesTotal    int `json:"messages_total"`
	RoleUser         int `json:"role_user"`
	RoleMerchant     int `json:"role_merchant"`
	RoleBot          int `json:"role_bot"`
	RoleSystem       int `json:"role_system"`
	FlagAutoReply    int `json:"flag_auto_reply"`
	FlagWaitBot      int `json:"flag_wait_bot"`
	FlagMedia        int `json:"flag_media"`
	FlagOrderCard    int `json:"flag_order_card"`
	FlagProductCard  int `json:"flag_product_card"`
	FlagComplaintCue int `json:"session_complaint_cue"`
	FlagHumanRequest int `json:"session_human_request"`

	RedactPhoneHits      int `json:"redact_phone_hits"`
	RedactTelHits        int `json:"redact_tel_hits"`
	RedactOrderIDHits    int `json:"redact_order_id_hits"`
	RedactContactHits    int `json:"redact_contact_hits"`
	RedactAddressHits    int `json:"redact_address_hits"`
	RedactAgentNameHit   int `json:"redact_agent_name_hits"`
	RedactMediaURLHits   int `json:"redact_media_url_hits"`
	RedactOrderCardHit   int `json:"redact_order_card_hits"`
	RedactProductCardHit int `json:"redact_product_card_hits"`

	// Ratios 是消息角色占比（%），与实施方案 §1.4 的 44/37/13/5 对照，
	// 用来判断解析有没有跑偏——如果 role_user 只有 5%，多半是格式没对上。
	Ratios map[string]float64 `json:"role_share_percent"`

	Warnings []string `json:"warnings,omitempty"`
}
