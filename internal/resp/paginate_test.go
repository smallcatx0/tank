package resp

import "testing"

func TestNewPagination_LimitNormalize(t *testing.T) {
	cases := []struct {
		name     string
		page     int
		limit    int
		wantPage int
		wantLmt  int
	}{
		{"正常值", 2, 20, 2, 20},
		{"limit为0归一为默认值", 1, 0, 1, DEF_LIMIT},
		{"limit为负数归一为默认值", 1, -5, 1, DEF_LIMIT},
		{"limit超上限截断", 1, 9999, 1, MAX_LIMIT},
		{"page小于1归一", 0, 10, 1, 10},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			pg := NewPagination(cs.page, cs.limit)
			if pg.Page != cs.wantPage {
				t.Errorf("Page = %d, want %d", pg.Page, cs.wantPage)
			}
			if pg.Limit != cs.wantLmt {
				t.Errorf("Limit = %d, want %d", pg.Limit, cs.wantLmt)
			}
			// limit 归一后，Calc 不应触发除零 panic
			pg.Calc(0)
		})
	}
}

func TestPagination_Calc(t *testing.T) {
	t.Run("total为0时Offset不为负", func(t *testing.T) {
		pg := NewPagination(1, 10)
		pg.Calc(0)
		if pg.Offset < 0 {
			t.Errorf("Offset = %d, want >= 0", pg.Offset)
		}
		if pg.Page != 1 {
			t.Errorf("Page = %d, want 1", pg.Page)
		}
		if pg.TotalPage != 0 {
			t.Errorf("TotalPage = %d, want 0", pg.TotalPage)
		}
	})
	t.Run("page越界钳制到末页", func(t *testing.T) {
		pg := NewPagination(100, 10)
		pg.Calc(95)
		if pg.Page != 10 || pg.TotalPage != 10 {
			t.Errorf("Page = %d, TotalPage = %d, want 10, 10", pg.Page, pg.TotalPage)
		}
		if pg.Offset != 90 {
			t.Errorf("Offset = %d, want 90", pg.Offset)
		}
	})
	t.Run("正常分页", func(t *testing.T) {
		pg := NewPagination(2, 10)
		pg.Calc(95)
		if pg.Page != 2 || pg.Offset != 10 || pg.TotalPage != 10 {
			t.Errorf("Page=%d Offset=%d TotalPage=%d, want 2 10 10", pg.Page, pg.Offset, pg.TotalPage)
		}
	})
}
