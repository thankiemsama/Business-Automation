package api

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

func (h *Handler) export(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	q := r.URL.Query()
	kind, format := q.Get("kind"), q.Get("format")
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "xlsx" {
		respond(w, 400, nil, core.Fail(400, "Định dạng không hợp lệ"))
		return
	}
	zone := time.FixedZone("ICT", 7*3600)
	now := time.Now().In(zone)
	from, to := now.AddDate(0, 0, -29), now
	var err error
	if q.Get("since") != "" {
		from, err = time.ParseInLocation(time.DateOnly, q.Get("since"), zone)
		if err != nil {
			respond(w, 400, nil, core.Fail(400, "Ngày bắt đầu không hợp lệ"))
			return
		}
	}
	if q.Get("to") != "" {
		to, err = time.ParseInLocation(time.DateOnly, q.Get("to"), zone)
		if err != nil {
			respond(w, 400, nil, core.Fail(400, "Ngày kết thúc không hợp lệ"))
			return
		}
	}
	if from.Format(time.DateOnly) > to.Format(time.DateOnly) {
		respond(w, 400, nil, core.Fail(400, "Khoảng ngày không hợp lệ"))
		return
	}
	to = time.Date(to.Year(), to.Month(), to.Day()+1, 0, 0, 0, 0, zone)
	rows, err := h.exportRows(user, kind, from, to, q.Get("since") != "")
	if err != nil {
		respond(w, 400, nil, err)
		return
	}
	var output bytes.Buffer
	contentType := "text/csv; charset=utf-8"
	if format == "csv" {
		output.WriteString("\uFEFF")
		encoder := csv.NewWriter(&output)
		for _, row := range rows {
			for i, cell := range row {
				row[i] = safeCSV(cell)
			}
			if err = encoder.Write(row); err != nil {
				break
			}
		}
		encoder.Flush()
		if err == nil {
			err = encoder.Error()
		}
	} else {
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		err = writeWorkbook(&output, rows, kind == "reports")
	}
	if err != nil {
		respond(w, 500, nil, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.%s", kind, format))
	_, _ = w.Write(output.Bytes())
}

func (h *Handler) exportRows(user core.User, kind string, from, to time.Time, filterStart bool) ([][]string, error) {
	if kind == "reports" {
		report, err := h.dashboard.Report(user, from, to)
		if err != nil {
			return nil, err
		}
		report.Total.Name = "Toàn đội"
		rows := [][]string{{"Nhân viên", "Phiếu mới", "Tồn hiện tại", "Phản hồi", "Phút phản hồi TB", "Đã xử lý", "Phút xử lý TB", "Lượt đánh giá", "Điểm TB"}}
		for _, row := range append([]service.ReportRow{report.Total}, report.Agents...) {
			rows = append(rows, []string{row.Name, strconv.Itoa(row.Opened), strconv.Itoa(row.Backlog), strconv.Itoa(row.Responses), fmt.Sprintf("%.2f", row.AverageResponseMinutes), strconv.Itoa(row.Resolved), fmt.Sprintf("%.2f", row.AverageResolutionMinutes), strconv.Itoa(row.Ratings), fmt.Sprintf("%.2f", row.AverageRating)})
		}
		return rows, nil
	}
	if kind != core.KindTickets && kind != core.KindCustomers {
		return nil, core.Fail(400, "Loại dữ liệu không hợp lệ")
	}
	records, err := h.resources.List(user, kind, "")
	if err != nil {
		return nil, err
	}
	rows := [][]string{{"ID", "Tiêu đề", "Trạng thái", "Ưu tiên", "Người phụ trách (ID)", "Nhãn", "Ngày tạo"}}
	if kind == core.KindCustomers {
		rows = [][]string{{"ID", "Tên", "Email", "Điện thoại", "Ngày tạo"}}
	}
	for _, record := range records {
		if (filterStart && record.CreatedAt.Before(from)) || !record.CreatedAt.Before(to) {
			continue
		}
		date := record.CreatedAt.Format(time.RFC3339)
		if kind == core.KindCustomers {
			rows = append(rows, []string{record.ID, record.Name, record.Email, record.Phone, date})
			continue
		}
		rows = append(rows, []string{record.ID, record.Subject, record.Status, record.Priority, record.AssigneeID, strings.Join(record.Tags, ", "), date})
	}
	return rows, nil
}

func safeCSV(cell string) string {
	trimmed := strings.TrimLeftFunc(cell, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || r == '\uFEFF' })
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + cell
	}
	return cell
}

func writeWorkbook(out *bytes.Buffer, rows [][]string, numericReport bool) error {
	archive := zip.NewWriter(out)
	files := map[string]string{
		"[Content_Types].xml":        `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels":                `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Nexa" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
	}
	for name, body := range files {
		entry, err := archive.Create(name)
		if err != nil {
			return err
		}
		if _, err = fmt.Fprint(entry, body); err != nil {
			return err
		}
	}
	entry, err := archive.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		return err
	}
	var sheet bytes.Buffer
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for rowIndex, row := range rows {
		sheet.WriteString("<row>")
		for columnIndex, cell := range row {
			if numericReport && rowIndex > 0 && columnIndex > 0 {
				sheet.WriteString("<c><v>" + cell + "</v></c>")
				continue
			}
			sheet.WriteString(`<c t="inlineStr"><is><t xml:space="preserve">`)
			if err := xml.EscapeText(&sheet, []byte(cell)); err != nil {
				return err
			}
			sheet.WriteString("</t></is></c>")
		}
		sheet.WriteString("</row>")
	}
	sheet.WriteString("</sheetData></worksheet>")
	if _, err = sheet.WriteTo(entry); err != nil {
		return err
	}
	return archive.Close()
}
