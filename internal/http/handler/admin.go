package handler

import (
	"net/http"
	"strings"
	"time"

	"bengkel/internal/http/response"
	"bengkel/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type branchInput struct {
	Code     string `json:"code" validate:"required,max=30"`
	Name     string `json:"name" validate:"required,max=150"`
	Address  string `json:"address"`
	Phone    string `json:"phone" validate:"max=30"`
	Timezone string `json:"timezone" validate:"required,max=60"`
	Currency string `json:"currency" validate:"required,len=3"`
	IsActive *bool  `json:"is_active" validate:"required"`
}

// ListBranches godoc
// @Summary Daftar cabang
// @Tags Branches
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /branches [get]
func (h *Handler) ListBranches(c *gin.Context) {
	var rows []model.Branch
	list(c, h.DB.Model(&model.Branch{}), &rows, []string{"code", "name", "address", "phone"})
}

// CreateBranch godoc
// @Summary Tambah cabang
// @Tags Branches
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body branchInput true "Cabang"
// @Success 201 {object} response.Envelope
// @Router /branches [post]
func (h *Handler) CreateBranch(c *gin.Context) {
	var in branchInput
	if !bind(c, &in) {
		return
	}
	row := model.Branch{
		Code: strings.ToUpper(strings.TrimSpace(in.Code)), Name: strings.TrimSpace(in.Name),
		Address: in.Address, Phone: in.Phone, Timezone: in.Timezone,
		Currency: strings.ToUpper(in.Currency), IsActive: *in.IsActive,
	}
	if err := h.DB.Create(&row).Error; err != nil {
		conflict(c, err, "BRANCH_EXISTS", "Kode cabang sudah digunakan")
		return
	}
	h.audit(c, "create", "branch", &row.ID, nil, row)
	response.Created(c, row)
}

// UpdateBranch godoc
// @Summary Ubah cabang
// @Tags Branches
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "ID cabang"
// @Param payload body branchInput true "Cabang"
// @Success 200 {object} response.Envelope
// @Router /branches/{id} [put]
func (h *Handler) UpdateBranch(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ID", "ID cabang tidak valid")
		return
	}
	var row model.Branch
	if err := h.DB.First(&row, "id=?", id).Error; err != nil {
		response.Error(c, http.StatusNotFound, "BRANCH_NOT_FOUND", "Cabang tidak ditemukan")
		return
	}
	var in branchInput
	if !bind(c, &in) {
		return
	}
	before := row
	row.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	row.Name, row.Address, row.Phone = strings.TrimSpace(in.Name), in.Address, in.Phone
	row.Timezone, row.Currency, row.IsActive = in.Timezone, strings.ToUpper(in.Currency), *in.IsActive
	if err := h.DB.Save(&row).Error; err != nil {
		conflict(c, err, "BRANCH_EXISTS", "Kode cabang sudah digunakan")
		return
	}
	h.audit(c, "update", "branch", &row.ID, before, row)
	response.OK(c, row)
}

type userInput struct {
	BranchID *uuid.UUID  `json:"branch_id"`
	Name     string      `json:"name" validate:"required,max=150"`
	Email    string      `json:"email" validate:"required,email,max=255"`
	Phone    string      `json:"phone" validate:"max=30"`
	Password string      `json:"password" validate:"required,min=8"`
	IsActive *bool       `json:"is_active" validate:"required"`
	RoleIDs  []uuid.UUID `json:"role_ids" validate:"required,min=1"`
}

type userUpdateInput struct {
	BranchID *uuid.UUID  `json:"branch_id"`
	Name     string      `json:"name" validate:"required,max=150"`
	Email    string      `json:"email" validate:"required,email,max=255"`
	Phone    string      `json:"phone" validate:"max=30"`
	Password string      `json:"password" validate:"omitempty,min=8"`
	IsActive *bool       `json:"is_active" validate:"required"`
	RoleIDs  []uuid.UUID `json:"role_ids" validate:"required,min=1"`
}

type userRow struct {
	ID          uuid.UUID  `json:"id"`
	BranchID    *uuid.UUID `json:"branch_id"`
	BranchName  string     `json:"branch_name"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Phone       string     `json:"phone"`
	IsActive    bool       `json:"is_active"`
	LastLoginAt *time.Time `json:"last_login_at"`
	Roles       []roleRef  `json:"roles"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type roleRef struct {
	ID   uuid.UUID `json:"id"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// ListUsers godoc
// @Summary Daftar pengguna dan role
// @Tags Users
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /users [get]
func (h *Handler) ListUsers(c *gin.Context) {
	page, per := positive(c.Query("page"), 1), positive(c.Query("per_page"), 10)
	if per > 100 {
		per = 100
	}
	query := h.DB.Model(&model.User{})
	if !hasRole(c, "owner") {
		query = query.Where("users.branch_id=?", branchID(c))
	}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(users.name) LIKE ? OR LOWER(users.email) LIKE ? OR LOWER(users.phone) LIKE ?", like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		serverError(c)
		return
	}
	var users []model.User
	if err := query.Order("users.created_at DESC").Offset((page - 1) * per).Limit(per).Find(&users).Error; err != nil {
		serverError(c)
		return
	}
	rows := make([]userRow, 0, len(users))
	for _, user := range users {
		var roles []roleRef
		h.DB.Table("roles r").Select("r.id,r.code,r.name").Joins("JOIN user_roles ur ON ur.role_id=r.id").Where("ur.user_id=?", user.ID).Order("r.name").Scan(&roles)
		var branchName string
		if user.BranchID != nil {
			h.DB.Model(&model.Branch{}).Where("id=?", user.BranchID).Select("name").Scan(&branchName)
		}
		rows = append(rows, userRow{user.ID, user.BranchID, branchName, user.Name, user.Email, user.Phone, user.IsActive, user.LastLoginAt, roles, user.CreatedAt, user.UpdatedAt})
	}
	response.Paginated(c, rows, page, per, total)
}

// CreateUser godoc
// @Summary Tambah pengguna
// @Tags Users
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body userInput true "Pengguna"
// @Success 201 {object} response.Envelope
// @Router /users [post]
func (h *Handler) CreateUser(c *gin.Context) {
	var in userInput
	if !bind(c, &in) {
		return
	}
	bid := branchID(c)
	if in.BranchID != nil {
		bid = *in.BranchID
	}
	if !h.branchExists(bid) {
		response.Error(c, http.StatusUnprocessableEntity, "BRANCH_NOT_FOUND", "Cabang tidak ditemukan")
		return
	}
	if !h.rolesExist(in.RoleIDs) {
		response.Error(c, http.StatusUnprocessableEntity, "ROLE_NOT_FOUND", "Salah satu role tidak ditemukan")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		serverError(c)
		return
	}
	row := model.User{BranchID: &bid, Name: strings.TrimSpace(in.Name), Email: strings.ToLower(strings.TrimSpace(in.Email)), Phone: in.Phone, PasswordHash: string(hash), IsActive: *in.IsActive}
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return replaceUserRoles(tx, row.ID, &bid, in.RoleIDs)
	})
	if err != nil {
		conflict(c, err, "USER_EXISTS", "Email sudah digunakan")
		return
	}
	h.audit(c, "create", "user", &row.ID, nil, map[string]any{"email": row.Email, "roles": in.RoleIDs})
	response.Created(c, row)
}

// UpdateUser godoc
// @Summary Ubah pengguna dan sinkronkan role
// @Tags Users
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "ID pengguna"
// @Param payload body userUpdateInput true "Pengguna"
// @Success 200 {object} response.Envelope
// @Router /users/{id} [put]
func (h *Handler) UpdateUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ID", "ID pengguna tidak valid")
		return
	}
	var row model.User
	query := h.DB.Where("id=?", id)
	if !hasRole(c, "owner") {
		query = query.Where("branch_id=?", branchID(c))
	}
	if err := query.First(&row).Error; err != nil {
		response.Error(c, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan")
		return
	}
	var in userUpdateInput
	if !bind(c, &in) {
		return
	}
	if id == userID(c) && !*in.IsActive {
		response.Error(c, http.StatusConflict, "SELF_DEACTIVATION", "Akun yang sedang digunakan tidak dapat dinonaktifkan")
		return
	}
	if h.isOwnerUser(id) && !h.roleIDsContainCode(in.RoleIDs, "owner") && h.activeOwnerCountExcluding(id) == 0 {
		response.Error(c, http.StatusConflict, "LAST_OWNER_PROTECTED", "Role owner terakhir tidak dapat dilepas")
		return
	}
	bid := branchID(c)
	if in.BranchID != nil {
		bid = *in.BranchID
	}
	if !h.branchExists(bid) || !h.rolesExist(in.RoleIDs) {
		response.Error(c, http.StatusUnprocessableEntity, "REFERENCE_INVALID", "Cabang atau role tidak ditemukan")
		return
	}
	before := map[string]any{"name": row.Name, "email": row.Email, "is_active": row.IsActive}
	row.BranchID, row.Name, row.Email, row.Phone, row.IsActive = &bid, strings.TrimSpace(in.Name), strings.ToLower(strings.TrimSpace(in.Email)), in.Phone, *in.IsActive
	if in.Password != "" {
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			serverError(c)
			return
		}
		row.PasswordHash = string(hash)
	}
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if err := replaceUserRoles(tx, row.ID, &bid, in.RoleIDs); err != nil {
			return err
		}
		if !row.IsActive || in.Password != "" {
			now := time.Now()
			return tx.Model(&model.RefreshToken{}).Where("user_id=? AND revoked_at IS NULL", row.ID).Update("revoked_at", now).Error
		}
		return nil
	})
	if err != nil {
		conflict(c, err, "USER_EXISTS", "Email sudah digunakan")
		return
	}
	h.audit(c, "update", "user", &row.ID, before, map[string]any{"name": row.Name, "email": row.Email, "is_active": row.IsActive, "roles": in.RoleIDs})
	response.OK(c, row)
}

// DeleteUser godoc
// @Summary Hapus pengguna
// @Tags Users
// @Security BearerAuth
// @Produce json
// @Param id path string true "ID pengguna"
// @Success 200 {object} response.Envelope
// @Router /users/{id} [delete]
func (h *Handler) DeleteUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ID", "ID pengguna tidak valid")
		return
	}
	if id == userID(c) {
		response.Error(c, http.StatusConflict, "SELF_DELETE", "Akun yang sedang digunakan tidak dapat dihapus")
		return
	}
	var row model.User
	query := h.DB.Where("id=?", id)
	if !hasRole(c, "owner") {
		query = query.Where("branch_id=?", branchID(c))
	}
	if err := query.First(&row).Error; err != nil {
		response.Error(c, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan")
		return
	}
	if h.isOwnerUser(id) && h.activeOwnerCountExcluding(id) == 0 {
		response.Error(c, http.StatusConflict, "LAST_OWNER_PROTECTED", "Pengguna owner terakhir tidak dapat dihapus")
		return
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Model(&model.RefreshToken{}).Where("user_id=? AND revoked_at IS NULL", row.ID).Update("revoked_at", now).Error; err != nil {
			return err
		}
		return tx.Delete(&row).Error
	}); err != nil {
		serverError(c)
		return
	}
	h.audit(c, "delete", "user", &row.ID, row, nil)
	response.OK(c, gin.H{"message": "Pengguna berhasil dihapus"})
}

type roleInput struct {
	Code          string      `json:"code" validate:"required,max=60"`
	Name          string      `json:"name" validate:"required,max=100"`
	Description   string      `json:"description"`
	PermissionIDs []uuid.UUID `json:"permission_ids" validate:"required,min=1"`
}

type roleRow struct {
	model.Role
	Permissions []model.Permission `json:"permissions"`
}

// ListRoles godoc
// @Summary Daftar role beserta permission
// @Tags Authorization
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /roles [get]
func (h *Handler) ListRoles(c *gin.Context) {
	var roles []model.Role
	if err := h.DB.Order("name").Find(&roles).Error; err != nil {
		serverError(c)
		return
	}
	rows := make([]roleRow, 0, len(roles))
	for _, role := range roles {
		var permissions []model.Permission
		h.DB.Table("permissions p").Select("p.*").Joins("JOIN role_permissions rp ON rp.permission_id=p.id").Where("rp.role_id=? AND p.deleted_at IS NULL", role.ID).Order("p.code").Scan(&permissions)
		rows = append(rows, roleRow{role, permissions})
	}
	response.OK(c, rows)
}

// ListPermissions godoc
// @Summary Katalog permission
// @Tags Authorization
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /permissions [get]
func (h *Handler) ListPermissions(c *gin.Context) {
	var rows []model.Permission
	if err := h.DB.Order("code").Find(&rows).Error; err != nil {
		serverError(c)
		return
	}
	response.OK(c, rows)
}

// CreateRole godoc
// @Summary Tambah role dan permission
// @Tags Authorization
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body roleInput true "Role"
// @Success 201 {object} response.Envelope
// @Router /roles [post]
func (h *Handler) CreateRole(c *gin.Context) {
	var in roleInput
	if !bind(c, &in) {
		return
	}
	if !h.permissionsExist(in.PermissionIDs) {
		response.Error(c, http.StatusUnprocessableEntity, "PERMISSION_NOT_FOUND", "Salah satu permission tidak ditemukan")
		return
	}
	row := model.Role{Code: strings.ToLower(strings.TrimSpace(in.Code)), Name: strings.TrimSpace(in.Name), Description: in.Description}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return replaceRolePermissions(tx, row.ID, in.PermissionIDs)
	})
	if err != nil {
		conflict(c, err, "ROLE_EXISTS", "Kode role sudah digunakan")
		return
	}
	h.audit(c, "create", "role", &row.ID, nil, map[string]any{"code": row.Code, "permissions": in.PermissionIDs})
	response.Created(c, row)
}

// UpdateRole godoc
// @Summary Ubah role dan sinkronkan permission
// @Tags Authorization
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "ID role"
// @Param payload body roleInput true "Role"
// @Success 200 {object} response.Envelope
// @Router /roles/{id} [put]
func (h *Handler) UpdateRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ID", "ID role tidak valid")
		return
	}
	var row model.Role
	if err := h.DB.First(&row, "id=?", id).Error; err != nil {
		response.Error(c, http.StatusNotFound, "ROLE_NOT_FOUND", "Role tidak ditemukan")
		return
	}
	if row.Code == "owner" {
		response.Error(c, http.StatusConflict, "OWNER_ROLE_PROTECTED", "Role owner tidak dapat diubah")
		return
	}
	var in roleInput
	if !bind(c, &in) {
		return
	}
	if !h.permissionsExist(in.PermissionIDs) {
		response.Error(c, http.StatusUnprocessableEntity, "PERMISSION_NOT_FOUND", "Salah satu permission tidak ditemukan")
		return
	}
	before := row
	row.Code, row.Name, row.Description = strings.ToLower(strings.TrimSpace(in.Code)), strings.TrimSpace(in.Name), in.Description
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return replaceRolePermissions(tx, row.ID, in.PermissionIDs)
	})
	if err != nil {
		conflict(c, err, "ROLE_EXISTS", "Kode role sudah digunakan")
		return
	}
	h.audit(c, "update", "role", &row.ID, before, map[string]any{"role": row, "permissions": in.PermissionIDs})
	response.OK(c, row)
}

// DeleteRole godoc
// @Summary Hapus role yang tidak digunakan
// @Tags Authorization
// @Security BearerAuth
// @Produce json
// @Param id path string true "ID role"
// @Success 200 {object} response.Envelope
// @Router /roles/{id} [delete]
func (h *Handler) DeleteRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ID", "ID role tidak valid")
		return
	}
	var row model.Role
	if err := h.DB.First(&row, "id=?", id).Error; err != nil {
		response.Error(c, http.StatusNotFound, "ROLE_NOT_FOUND", "Role tidak ditemukan")
		return
	}
	if row.Code == "owner" {
		response.Error(c, http.StatusConflict, "OWNER_ROLE_PROTECTED", "Role owner tidak dapat dihapus")
		return
	}
	var assigned int64
	h.DB.Table("user_roles").Where("role_id=?", id).Count(&assigned)
	if assigned > 0 {
		response.Error(c, http.StatusConflict, "ROLE_IN_USE", "Role masih digunakan oleh pengguna")
		return
	}
	if err := h.DB.Delete(&row).Error; err != nil {
		serverError(c)
		return
	}
	h.audit(c, "delete", "role", &row.ID, row, nil)
	response.OK(c, gin.H{"message": "Role berhasil dihapus"})
}

func (h *Handler) branchExists(id uuid.UUID) bool {
	var count int64
	h.DB.Model(&model.Branch{}).Where("id=? AND is_active=true", id).Count(&count)
	return count == 1
}

func (h *Handler) rolesExist(ids []uuid.UUID) bool {
	if len(ids) == 0 {
		return false
	}
	unique := uniqueIDs(ids)
	if len(unique) != len(ids) {
		return false
	}
	var count int64
	h.DB.Model(&model.Role{}).Where("id IN ?", ids).Count(&count)
	return count == int64(len(unique))
}

func (h *Handler) permissionsExist(ids []uuid.UUID) bool {
	if len(ids) == 0 {
		return false
	}
	unique := uniqueIDs(ids)
	if len(unique) != len(ids) {
		return false
	}
	var count int64
	h.DB.Model(&model.Permission{}).Where("id IN ?", ids).Count(&count)
	return count == int64(len(unique))
}

func (h *Handler) isOwnerUser(id uuid.UUID) bool {
	var count int64
	h.DB.Table("user_roles ur").Joins("JOIN roles r ON r.id=ur.role_id").Where("ur.user_id=? AND r.code='owner' AND r.deleted_at IS NULL", id).Count(&count)
	return count > 0
}

func (h *Handler) roleIDsContainCode(ids []uuid.UUID, code string) bool {
	var count int64
	h.DB.Model(&model.Role{}).Where("id IN ? AND code=?", ids, code).Count(&count)
	return count > 0
}

func (h *Handler) activeOwnerCountExcluding(id uuid.UUID) int64 {
	var count int64
	h.DB.Table("users u").Distinct("u.id").Joins("JOIN user_roles ur ON ur.user_id=u.id").Joins("JOIN roles r ON r.id=ur.role_id").
		Where("r.code='owner' AND u.id<>? AND u.is_active=true AND u.deleted_at IS NULL AND r.deleted_at IS NULL", id).Count(&count)
	return count
}

func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]bool, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id != uuid.Nil && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func replaceUserRoles(tx *gorm.DB, userID uuid.UUID, branchID *uuid.UUID, roleIDs []uuid.UUID) error {
	if err := tx.Where("user_id=?", userID).Delete(&model.UserRole{}).Error; err != nil {
		return err
	}
	for _, roleID := range uniqueIDs(roleIDs) {
		if err := tx.Create(&model.UserRole{UserID: userID, RoleID: roleID, BranchID: branchID, CreatedAt: time.Now()}).Error; err != nil {
			return err
		}
	}
	return nil
}

func replaceRolePermissions(tx *gorm.DB, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	if err := tx.Exec("DELETE FROM role_permissions WHERE role_id=?", roleID).Error; err != nil {
		return err
	}
	for _, permissionID := range uniqueIDs(permissionIDs) {
		if err := tx.Exec("INSERT INTO role_permissions(role_id,permission_id,created_at) VALUES (?,?,now())", roleID, permissionID).Error; err != nil {
			return err
		}
	}
	return nil
}

func hasRole(c *gin.Context, required string) bool {
	value, _ := c.Get("roles")
	roles, _ := value.([]string)
	for _, role := range roles {
		if role == required {
			return true
		}
	}
	return false
}
