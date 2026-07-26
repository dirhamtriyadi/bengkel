package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"bengkel/internal/http/response"
	"bengkel/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type accountInput struct {
	Code     string     `json:"code" validate:"required,max=30"`
	Name     string     `json:"name" validate:"required,max=150"`
	Type     string     `json:"type" validate:"required,oneof=asset liability equity revenue expense"`
	ParentID *uuid.UUID `json:"parent_id"`
	IsActive *bool      `json:"is_active" validate:"required"`
}

// ListAccounts godoc
// @Summary Daftar chart of accounts
// @Tags Accounting
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /accounts [get]
func (h *Handler) ListAccounts(c *gin.Context) {
	var rows []model.Account
	list(c, h.DB.Model(&model.Account{}).Where("branch_id=?", branchID(c)), &rows, []string{"code", "name", "type"})
}

// CreateAccount godoc
// @Summary Tambah akun COA
// @Tags Accounting
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body accountInput true "Akun"
// @Success 201 {object} response.Envelope
// @Router /accounts [post]
func (h *Handler) CreateAccount(c *gin.Context) {
	var in accountInput
	if !bind(c, &in) {
		return
	}
	if in.ParentID != nil && !h.accountExists(*in.ParentID, branchID(c), false) {
		response.Error(c, http.StatusUnprocessableEntity, "PARENT_ACCOUNT_NOT_FOUND", "Akun induk tidak ditemukan")
		return
	}
	bid := branchID(c)
	row := model.Account{BranchID: &bid, Code: strings.TrimSpace(in.Code), Name: strings.TrimSpace(in.Name), Type: in.Type, ParentID: in.ParentID, IsActive: *in.IsActive}
	if err := h.DB.Create(&row).Error; err != nil {
		conflict(c, err, "ACCOUNT_EXISTS", "Kode akun sudah digunakan")
		return
	}
	h.audit(c, "create", "account", &row.ID, nil, row)
	response.Created(c, row)
}

// UpdateAccount godoc
// @Summary Ubah akun COA
// @Tags Accounting
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "ID akun"
// @Param payload body accountInput true "Akun"
// @Success 200 {object} response.Envelope
// @Router /accounts/{id} [put]
func (h *Handler) UpdateAccount(c *gin.Context) {
	var row model.Account
	if !h.findScoped(c, &row) {
		return
	}
	var in accountInput
	if !bind(c, &in) {
		return
	}
	if in.ParentID != nil {
		if *in.ParentID == row.ID {
			response.Error(c, http.StatusUnprocessableEntity, "PARENT_ACCOUNT_INVALID", "Akun tidak dapat menjadi induk dirinya sendiri")
			return
		}
		if !h.accountExists(*in.ParentID, branchID(c), false) {
			response.Error(c, http.StatusUnprocessableEntity, "PARENT_ACCOUNT_NOT_FOUND", "Akun induk tidak ditemukan")
			return
		}
		if h.accountParentCycle(row.ID, *in.ParentID) {
			response.Error(c, http.StatusUnprocessableEntity, "ACCOUNT_CYCLE", "Relasi akun induk akan membentuk siklus")
			return
		}
	}
	before := row
	row.Code, row.Name, row.Type, row.ParentID, row.IsActive = strings.TrimSpace(in.Code), strings.TrimSpace(in.Name), in.Type, in.ParentID, *in.IsActive
	if err := h.DB.Save(&row).Error; err != nil {
		conflict(c, err, "ACCOUNT_EXISTS", "Kode akun sudah digunakan")
		return
	}
	h.audit(c, "update", "account", &row.ID, before, row)
	response.OK(c, row)
}

// DeleteAccount godoc
// @Summary Hapus akun yang belum digunakan
// @Tags Accounting
// @Security BearerAuth
// @Produce json
// @Param id path string true "ID akun"
// @Success 200 {object} response.Envelope
// @Router /accounts/{id} [delete]
func (h *Handler) DeleteAccount(c *gin.Context) {
	var row model.Account
	if !h.findScoped(c, &row) {
		return
	}
	var used, children int64
	h.DB.Model(&model.JournalLine{}).Where("account_id=?", row.ID).Count(&used)
	h.DB.Model(&model.Account{}).Where("parent_id=?", row.ID).Count(&children)
	if used > 0 || children > 0 {
		response.Error(c, http.StatusConflict, "ACCOUNT_IN_USE", "Akun memiliki jurnal atau akun turunan dan tidak dapat dihapus")
		return
	}
	if err := h.DB.Delete(&row).Error; err != nil {
		serverError(c)
		return
	}
	h.audit(c, "delete", "account", &row.ID, row, nil)
	response.OK(c, gin.H{"message": "Akun berhasil dihapus"})
}

type journalLineInput struct {
	AccountID   uuid.UUID `json:"account_id" validate:"required"`
	Description string    `json:"description"`
	Debit       int64     `json:"debit" validate:"gte=0"`
	Credit      int64     `json:"credit" validate:"gte=0"`
}

type journalInput struct {
	EntryDate   string             `json:"entry_date" validate:"required"`
	Description string             `json:"description" validate:"required,max=500"`
	Lines       []journalLineInput `json:"lines" validate:"required,min=2"`
}

type journalListRow struct {
	ID            uuid.UUID `json:"id"`
	Number        string    `json:"number"`
	EntryDate     time.Time `json:"entry_date"`
	Description   string    `json:"description"`
	ReferenceType string    `json:"reference_type"`
	Status        string    `json:"status"`
	TotalDebit    int64     `json:"total_debit"`
	TotalCredit   int64     `json:"total_credit"`
	CreatedAt     time.Time `json:"created_at"`
}

type journalDetail struct {
	model.JournalEntry
	Lines []journalDetailLine `json:"lines"`
}

type journalDetailLine struct {
	model.JournalLine
	AccountCode string `json:"account_code"`
	AccountName string `json:"account_name"`
}

// ListJournals godoc
// @Summary Daftar journal entry
// @Tags Accounting
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /journals [get]
func (h *Handler) ListJournals(c *gin.Context) {
	query := h.DB.Table("journal_entries je").
		Select("je.id,je.number,je.entry_date,je.description,je.reference_type,je.status,je.created_at,COALESCE(SUM(jl.debit),0) total_debit,COALESCE(SUM(jl.credit),0) total_credit").
		Joins("LEFT JOIN journal_lines jl ON jl.journal_entry_id=je.id AND jl.deleted_at IS NULL").
		Where("je.branch_id=? AND je.deleted_at IS NULL", branchID(c)).
		Group("je.id")
	var rows []journalListRow
	list(c, query, &rows, []string{"je.number", "je.description", "je.reference_type", "je.status"})
}

// JournalDetail godoc
// @Summary Detail journal entry dan baris
// @Tags Accounting
// @Security BearerAuth
// @Produce json
// @Param id path string true "ID jurnal"
// @Success 200 {object} response.Envelope
// @Router /journals/{id} [get]
func (h *Handler) JournalDetail(c *gin.Context) {
	var entry model.JournalEntry
	if !h.findScoped(c, &entry) {
		return
	}
	var lines []journalDetailLine
	if err := h.DB.Table("journal_lines jl").Select("jl.*,a.code account_code,a.name account_name").
		Joins("JOIN accounts a ON a.id=jl.account_id").
		Where("jl.journal_entry_id=? AND jl.deleted_at IS NULL", entry.ID).Order("jl.created_at").Scan(&lines).Error; err != nil {
		serverError(c)
		return
	}
	response.OK(c, journalDetail{entry, lines})
}

// CreateJournal godoc
// @Summary Buat jurnal manual sebagai draft
// @Tags Accounting
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body journalInput true "Journal entry"
// @Success 201 {object} response.Envelope
// @Router /journals [post]
func (h *Handler) CreateJournal(c *gin.Context) {
	var in journalInput
	if !bind(c, &in) {
		return
	}
	date, lines, ok := h.prepareJournal(c, in)
	if !ok {
		return
	}
	entry := model.JournalEntry{BranchID: branchID(c), Number: number("JE"), EntryDate: date, Description: strings.TrimSpace(in.Description), ReferenceType: "manual", Status: "draft", CreatedBy: userID(c)}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		for i := range lines {
			lines[i].JournalEntryID = entry.ID
		}
		return tx.Create(&lines).Error
	})
	if err != nil {
		serverError(c)
		return
	}
	h.audit(c, "create", "journal_entry", &entry.ID, nil, map[string]any{"entry": entry, "lines": lines})
	response.Created(c, journalDetail{entry, toJournalDetails(lines)})
}

// UpdateJournal godoc
// @Summary Ubah jurnal draft
// @Tags Accounting
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "ID jurnal"
// @Param payload body journalInput true "Journal entry"
// @Success 200 {object} response.Envelope
// @Router /journals/{id} [put]
func (h *Handler) UpdateJournal(c *gin.Context) {
	var entry model.JournalEntry
	if !h.findScoped(c, &entry) {
		return
	}
	if entry.Status != "draft" {
		response.Error(c, http.StatusConflict, "JOURNAL_IMMUTABLE", "Hanya jurnal draft yang dapat diubah")
		return
	}
	var in journalInput
	if !bind(c, &in) {
		return
	}
	date, lines, ok := h.prepareJournal(c, in)
	if !ok {
		return
	}
	before := entry
	entry.EntryDate, entry.Description = date, strings.TrimSpace(in.Description)
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&entry).Error; err != nil {
			return err
		}
		if err := tx.Where("journal_entry_id=?", entry.ID).Delete(&model.JournalLine{}).Error; err != nil {
			return err
		}
		for i := range lines {
			lines[i].JournalEntryID = entry.ID
		}
		return tx.Create(&lines).Error
	})
	if err != nil {
		serverError(c)
		return
	}
	h.audit(c, "update", "journal_entry", &entry.ID, before, map[string]any{"entry": entry, "lines": lines})
	response.OK(c, journalDetail{entry, toJournalDetails(lines)})
}

// PostJournal godoc
// @Summary Posting jurnal secara immutable
// @Tags Accounting
// @Security BearerAuth
// @Produce json
// @Param id path string true "ID jurnal"
// @Success 200 {object} response.Envelope
// @Router /journals/{id}/post [post]
func (h *Handler) PostJournal(c *gin.Context) {
	var entry model.JournalEntry
	if !h.findScoped(c, &entry) {
		return
	}
	if entry.Status != "draft" {
		response.Error(c, http.StatusConflict, "JOURNAL_NOT_DRAFT", "Jurnal bukan berstatus draft")
		return
	}
	var debit, credit int64
	h.DB.Model(&model.JournalLine{}).Where("journal_entry_id=?", entry.ID).Select("COALESCE(SUM(debit),0)").Scan(&debit)
	h.DB.Model(&model.JournalLine{}).Where("journal_entry_id=?", entry.ID).Select("COALESCE(SUM(credit),0)").Scan(&credit)
	if debit == 0 || debit != credit {
		response.Error(c, http.StatusUnprocessableEntity, "JOURNAL_UNBALANCED", "Total debit dan kredit harus sama dan lebih dari nol")
		return
	}
	if err := h.DB.Model(&entry).Update("status", "posted").Error; err != nil {
		serverError(c)
		return
	}
	entry.Status = "posted"
	h.audit(c, "post", "journal_entry", &entry.ID, map[string]any{"status": "draft"}, map[string]any{"status": "posted"})
	response.OK(c, entry)
}

// ReverseJournal godoc
// @Summary Buat jurnal pembalik
// @Tags Accounting
// @Security BearerAuth
// @Produce json
// @Param id path string true "ID jurnal"
// @Success 201 {object} response.Envelope
// @Router /journals/{id}/reverse [post]
func (h *Handler) ReverseJournal(c *gin.Context) {
	var original model.JournalEntry
	if !h.findScoped(c, &original) {
		return
	}
	if original.Status != "posted" {
		response.Error(c, http.StatusConflict, "JOURNAL_NOT_POSTED", "Hanya jurnal terposting yang dapat dibalik")
		return
	}
	var existing int64
	h.DB.Model(&model.JournalEntry{}).Where("branch_id=? AND reference_type='journal_reversal' AND reference_id=? AND status='posted'", branchID(c), original.ID).Count(&existing)
	if existing > 0 {
		response.Error(c, http.StatusConflict, "JOURNAL_ALREADY_REVERSED", "Jurnal ini sudah memiliki jurnal pembalik")
		return
	}
	var originalLines []model.JournalLine
	if err := h.DB.Where("journal_entry_id=?", original.ID).Find(&originalLines).Error; err != nil {
		serverError(c)
		return
	}
	entry := model.JournalEntry{BranchID: branchID(c), Number: number("REV"), EntryDate: time.Now(), Description: "Pembalik " + original.Number + " — " + original.Description, ReferenceType: "journal_reversal", ReferenceID: &original.ID, Status: "posted", CreatedBy: userID(c)}
	lines := make([]model.JournalLine, 0, len(originalLines))
	for _, line := range originalLines {
		lines = append(lines, model.JournalLine{AccountID: line.AccountID, Description: "Pembalik: " + line.Description, Debit: line.Credit, Credit: line.Debit})
	}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		// The immutability trigger permits initial lines only while the entry is draft.
		entry.Status = "draft"
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		for i := range lines {
			lines[i].JournalEntryID = entry.ID
		}
		if err := tx.Create(&lines).Error; err != nil {
			return err
		}
		return tx.Model(&entry).Update("status", "posted").Error
	})
	if err != nil {
		serverError(c)
		return
	}
	entry.Status = "posted"
	h.audit(c, "reverse", "journal_entry", &entry.ID, original, map[string]any{"entry": entry, "lines": lines})
	response.Created(c, journalDetail{entry, toJournalDetails(lines)})
}

// DeleteJournal godoc
// @Summary Hapus jurnal draft
// @Tags Accounting
// @Security BearerAuth
// @Produce json
// @Param id path string true "ID jurnal"
// @Success 200 {object} response.Envelope
// @Router /journals/{id} [delete]
func (h *Handler) DeleteJournal(c *gin.Context) {
	var entry model.JournalEntry
	if !h.findScoped(c, &entry) {
		return
	}
	if entry.Status != "draft" {
		response.Error(c, http.StatusConflict, "JOURNAL_IMMUTABLE", "Hanya jurnal draft yang dapat dihapus")
		return
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("journal_entry_id=?", entry.ID).Delete(&model.JournalLine{}).Error; err != nil {
			return err
		}
		return tx.Delete(&entry).Error
	}); err != nil {
		serverError(c)
		return
	}
	h.audit(c, "delete", "journal_entry", &entry.ID, entry, nil)
	response.OK(c, gin.H{"message": "Jurnal draft berhasil dihapus"})
}

// ListPayments godoc
// @Summary Daftar pembayaran
// @Tags Payments
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /payments [get]
func (h *Handler) ListPayments(c *gin.Context) {
	query := h.DB.Table("payments p").Select("p.*,s.number sale_number,s.grand_total sale_total").
		Joins("JOIN sales s ON s.id=p.sale_id").
		Where("p.branch_id=? AND p.deleted_at IS NULL", branchID(c))
	rows := make([]map[string]any, 0)
	list(c, query, &rows, []string{"s.number", "p.method", "p.provider", "p.provider_reference", "p.status"})
}

// PaymentDetail godoc
// @Summary Detail pembayaran
// @Tags Payments
// @Security BearerAuth
// @Produce json
// @Param id path string true "ID pembayaran"
// @Success 200 {object} response.Envelope
// @Router /payments/{id} [get]
func (h *Handler) PaymentDetail(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ID", "ID pembayaran tidak valid")
		return
	}
	var row map[string]any
	err = h.DB.Table("payments p").Select("p.*,s.number sale_number,s.grand_total sale_total,s.status sale_status").
		Joins("JOIN sales s ON s.id=p.sale_id").Where("p.id=? AND p.branch_id=? AND p.deleted_at IS NULL", id, branchID(c)).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.Error(c, http.StatusNotFound, "PAYMENT_NOT_FOUND", "Pembayaran tidak ditemukan")
		return
	}
	if err != nil {
		serverError(c)
		return
	}
	response.OK(c, row)
}

type reportAccountRow struct {
	ID      uuid.UUID `json:"id"`
	Code    string    `json:"code"`
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Debit   int64     `json:"debit"`
	Credit  int64     `json:"credit"`
	Balance int64     `json:"balance"`
}

// TrialBalance godoc
// @Summary Laporan neraca saldo
// @Tags Reports
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /reports/trial-balance [get]
func (h *Handler) TrialBalance(c *gin.Context) {
	to := reportEndDate(c)
	rows := make([]reportAccountRow, 0)
	err := h.DB.Table("accounts a").Select(`a.id,a.code,a.name,a.type,
		COALESCE(SUM(CASE WHEN je.id IS NOT NULL THEN jl.debit ELSE 0 END),0) debit,
		COALESCE(SUM(CASE WHEN je.id IS NOT NULL THEN jl.credit ELSE 0 END),0) credit,
		CASE WHEN a.type IN ('asset','expense')
		THEN COALESCE(SUM(CASE WHEN je.id IS NOT NULL THEN jl.debit-jl.credit ELSE 0 END),0)
		ELSE COALESCE(SUM(CASE WHEN je.id IS NOT NULL THEN jl.credit-jl.debit ELSE 0 END),0) END balance`).
		Joins("LEFT JOIN journal_lines jl ON jl.account_id=a.id AND jl.deleted_at IS NULL").
		Joins("LEFT JOIN journal_entries je ON je.id=jl.journal_entry_id AND je.status='posted' AND je.entry_date<=?", to).
		Where("a.branch_id=? AND a.deleted_at IS NULL", branchID(c)).Group("a.id").Order("a.code").Scan(&rows).Error
	if err != nil {
		serverError(c)
		return
	}
	var debit, credit int64
	for _, row := range rows {
		debit += row.Debit
		credit += row.Credit
	}
	response.OK(c, gin.H{"to": to, "accounts": rows, "total_debit": debit, "total_credit": credit, "balanced": debit == credit})
}

// BalanceSheet godoc
// @Summary Laporan posisi keuangan
// @Tags Reports
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /reports/balance-sheet [get]
func (h *Handler) BalanceSheet(c *gin.Context) {
	to := reportEndDate(c)
	rows := make([]reportAccountRow, 0)
	err := h.DB.Table("accounts a").Select(`a.id,a.code,a.name,a.type,
		COALESCE(SUM(CASE WHEN je.id IS NOT NULL THEN jl.debit ELSE 0 END),0) debit,
		COALESCE(SUM(CASE WHEN je.id IS NOT NULL THEN jl.credit ELSE 0 END),0) credit,
		CASE WHEN a.type='asset'
		THEN COALESCE(SUM(CASE WHEN je.id IS NOT NULL THEN jl.debit-jl.credit ELSE 0 END),0)
		ELSE COALESCE(SUM(CASE WHEN je.id IS NOT NULL THEN jl.credit-jl.debit ELSE 0 END),0) END balance`).
		Joins("LEFT JOIN journal_lines jl ON jl.account_id=a.id AND jl.deleted_at IS NULL").
		Joins("LEFT JOIN journal_entries je ON je.id=jl.journal_entry_id AND je.status='posted' AND je.entry_date<=?", to).
		Where("a.branch_id=? AND a.deleted_at IS NULL AND a.type IN ('asset','liability','equity')", branchID(c)).
		Group("a.id").Order("a.code").Scan(&rows).Error
	if err != nil {
		serverError(c)
		return
	}
	var assets, liabilities, equity, retained int64
	for _, row := range rows {
		switch row.Type {
		case "asset":
			assets += row.Balance
		case "liability":
			liabilities += row.Balance
		case "equity":
			equity += row.Balance
		}
	}
	h.DB.Table("journal_lines jl").Select(`COALESCE(SUM(jl.credit-jl.debit),0)`).
		Joins("JOIN journal_entries je ON je.id=jl.journal_entry_id").Joins("JOIN accounts a ON a.id=jl.account_id").
		Where("je.branch_id=? AND je.status='posted' AND je.entry_date<=? AND a.type IN ('revenue','expense')", branchID(c), to).Scan(&retained)
	response.OK(c, gin.H{"to": to, "accounts": rows, "total_assets": assets, "total_liabilities": liabilities, "total_equity": equity, "retained_earnings": retained, "total_liabilities_equity": liabilities + equity + retained})
}

// CashFlow godoc
// @Summary Laporan arus kas
// @Tags Reports
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /reports/cash-flow [get]
func (h *Handler) CashFlow(c *gin.Context) {
	from, to := reportPeriod(c)
	type cashRow struct {
		Date          time.Time `json:"date"`
		JournalNumber string    `json:"journal_number"`
		Description   string    `json:"description"`
		ReferenceType string    `json:"reference_type"`
		AccountCode   string    `json:"account_code"`
		AccountName   string    `json:"account_name"`
		Inflow        int64     `json:"inflow"`
		Outflow       int64     `json:"outflow"`
	}
	rows := make([]cashRow, 0)
	err := h.DB.Table("journal_lines jl").Select("je.entry_date date,je.number journal_number,je.description,je.reference_type,a.code account_code,a.name account_name,jl.debit inflow,jl.credit outflow").
		Joins("JOIN journal_entries je ON je.id=jl.journal_entry_id").Joins("JOIN accounts a ON a.id=jl.account_id").
		Where("je.branch_id=? AND je.status='posted' AND je.entry_date BETWEEN ? AND ? AND a.type='asset' AND (a.code='1101' OR LOWER(a.name) LIKE '%kas%' OR LOWER(a.name) LIKE '%bank%')", branchID(c), from, to).
		Order("je.entry_date,je.number").Scan(&rows).Error
	if err != nil {
		serverError(c)
		return
	}
	var inflow, outflow int64
	for _, row := range rows {
		inflow += row.Inflow
		outflow += row.Outflow
	}
	response.OK(c, gin.H{"from": from, "to": to, "transactions": rows, "total_inflow": inflow, "total_outflow": outflow, "net_cash_flow": inflow - outflow})
}

// SalesReport godoc
// @Summary Laporan penjualan per hari dan metode
// @Tags Reports
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /reports/sales [get]
func (h *Handler) SalesReport(c *gin.Context) {
	from, to := reportPeriod(c)
	type row struct {
		Date         time.Time `json:"date"`
		Method       string    `json:"method"`
		Transactions int64     `json:"transactions"`
		GrossSales   int64     `json:"gross_sales"`
		GatewayFee   int64     `json:"gateway_fee"`
	}
	rows := make([]row, 0)
	err := h.DB.Table("sales s").Select("s.paid_at::date date,p.method,COUNT(DISTINCT s.id) transactions,SUM(s.grand_total) gross_sales,SUM(p.fee) gateway_fee").
		Joins("JOIN payments p ON p.sale_id=s.id AND p.status='paid'").
		Where("s.branch_id=? AND s.status='paid' AND s.paid_at::date BETWEEN ? AND ?", branchID(c), from, to).
		Group("s.paid_at::date,p.method").Order("date DESC,p.method").Scan(&rows).Error
	if err != nil {
		serverError(c)
		return
	}
	response.OK(c, gin.H{"from": from, "to": to, "rows": rows})
}

// InventoryValuation godoc
// @Summary Laporan nilai persediaan
// @Tags Reports
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /reports/inventory-valuation [get]
func (h *Handler) InventoryValuation(c *gin.Context) {
	type row struct {
		ProductID uuid.UUID `json:"product_id"`
		SKU       string    `json:"sku"`
		Name      string    `json:"name"`
		Quantity  float64   `json:"quantity"`
		UnitCost  int64     `json:"unit_cost"`
		Value     float64   `json:"value"`
	}
	rows := make([]row, 0)
	err := h.DB.Table("inventory_balances ib").Select("p.id product_id,p.sku,p.name,ib.quantity,p.cost_price unit_cost,ib.quantity*p.cost_price value").
		Joins("JOIN products p ON p.id=ib.product_id").Where("ib.branch_id=? AND p.type='part' AND p.deleted_at IS NULL", branchID(c)).
		Order("p.name").Scan(&rows).Error
	if err != nil {
		serverError(c)
		return
	}
	var total float64
	for _, row := range rows {
		total += row.Value
	}
	response.OK(c, gin.H{"rows": rows, "total_value": total})
}

func (h *Handler) prepareJournal(c *gin.Context, in journalInput) (time.Time, []model.JournalLine, bool) {
	date, err := time.Parse("2006-01-02", in.EntryDate)
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "ENTRY_DATE_INVALID", "Tanggal jurnal harus berformat YYYY-MM-DD")
		return time.Time{}, nil, false
	}
	lines := make([]model.JournalLine, 0, len(in.Lines))
	var debit, credit int64
	for index, item := range in.Lines {
		if item.AccountID == uuid.Nil || !h.accountExists(item.AccountID, branchID(c), true) {
			response.Error(c, http.StatusUnprocessableEntity, "ACCOUNT_NOT_FOUND", "Akun pada baris jurnal tidak ditemukan atau tidak aktif", response.FieldError{Field: "lines", Rule: "account", Message: "Baris " + numberString(index+1) + " memiliki akun tidak valid"})
			return time.Time{}, nil, false
		}
		if (item.Debit <= 0 && item.Credit <= 0) || (item.Debit > 0 && item.Credit > 0) {
			response.Error(c, http.StatusUnprocessableEntity, "JOURNAL_LINE_INVALID", "Setiap baris harus memiliki tepat satu nilai debit atau kredit")
			return time.Time{}, nil, false
		}
		debit += item.Debit
		credit += item.Credit
		lines = append(lines, model.JournalLine{AccountID: item.AccountID, Description: item.Description, Debit: item.Debit, Credit: item.Credit})
	}
	if debit == 0 || debit != credit {
		response.Error(c, http.StatusUnprocessableEntity, "JOURNAL_UNBALANCED", "Total debit dan kredit harus sama dan lebih dari nol")
		return time.Time{}, nil, false
	}
	return date, lines, true
}

func (h *Handler) accountExists(id, branchID uuid.UUID, activeOnly bool) bool {
	query := h.DB.Model(&model.Account{}).Where("id=? AND branch_id=?", id, branchID)
	if activeOnly {
		query = query.Where("is_active=true")
	}
	var count int64
	query.Count(&count)
	return count == 1
}

func (h *Handler) accountParentCycle(accountID, parentID uuid.UUID) bool {
	current := parentID
	for range 100 {
		if current == accountID {
			return true
		}
		var account model.Account
		if err := h.DB.Select("id", "parent_id").First(&account, "id=?", current).Error; err != nil || account.ParentID == nil {
			return false
		}
		current = *account.ParentID
	}
	return true
}

func toJournalDetails(lines []model.JournalLine) []journalDetailLine {
	out := make([]journalDetailLine, 0, len(lines))
	for _, line := range lines {
		out = append(out, journalDetailLine{JournalLine: line})
	}
	return out
}

func reportPeriod(c *gin.Context) (string, string) {
	now := time.Now()
	from := c.DefaultQuery("from", time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02"))
	to := c.DefaultQuery("to", now.Format("2006-01-02"))
	return from, to
}

func reportEndDate(c *gin.Context) string {
	return c.DefaultQuery("to", time.Now().Format("2006-01-02"))
}

func numberString(value int) string {
	const digits = "0123456789"
	if value >= 0 && value < 10 {
		return string(digits[value])
	}
	return "?"
}
