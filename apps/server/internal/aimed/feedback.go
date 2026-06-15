package aimed

import "gorm.io/gorm"

// §8.10.5 踩反馈固定 7 项原因（逐字一致）。
var FeedbackReasons = []string{"不准确", "引用错误", "没有回答问题", "格式不好", "内容太少", "内容太长", "其他"}

// IsValidFeedbackReason 校验踩原因是否为固定 7 项之一。
func IsValidFeedbackReason(r string) bool {
	for _, x := range FeedbackReasons {
		if x == r {
			return true
		}
	}
	return false
}

// WriteFeedback 写一条反馈（design D8：feedbacks owner=c04，subject_type∈{message,translation_job}）。
// AIMed 赞/踩按 subject_type=message、subject_id=message_id 写入，租户隔离。
func WriteFeedback(db *gorm.DB, tenantID, userID, subjectType, subjectID, rating, reason, comment string) error {
	return db.Exec(
		`INSERT INTO feedbacks (tenant_id, user_id, subject_type, subject_id, rating, reason, comment)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tenantID, userID, subjectType, subjectID, rating, nullStr(reason), nullStr(comment),
	).Error
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
