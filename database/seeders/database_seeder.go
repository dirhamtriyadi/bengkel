package seeders

import (
	"encoding/json"
	"fmt"
	"time"

	"bengkel/internal/model"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Run is the equivalent of Laravel's DatabaseSeeder::run().
// Every child seeder is deterministic and safe to execute repeatedly.
func Run(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		branch, err := branchSeeder(tx)
		if err != nil {
			return fmt.Errorf("BranchSeeder: %w", err)
		}
		if err := rolePermissionSeeder(tx); err != nil {
			return fmt.Errorf("RolePermissionSeeder: %w", err)
		}
		users, err := userSeeder(tx, branch)
		if err != nil {
			return fmt.Errorf("UserSeeder: %w", err)
		}
		if err := accountSeeder(tx, branch); err != nil {
			return fmt.Errorf("AccountSeeder: %w", err)
		}
		products, err := productSeeder(tx, branch)
		if err != nil {
			return fmt.Errorf("ProductSeeder: %w", err)
		}
		if err := inventorySeeder(tx, branch, users[0], products); err != nil {
			return fmt.Errorf("InventorySeeder: %w", err)
		}
		if err := openingJournalSeeder(tx, branch, users[0], products); err != nil {
			return fmt.Errorf("OpeningJournalSeeder: %w", err)
		}
		if err := customerVehicleSeeder(tx, branch); err != nil {
			return fmt.Errorf("CustomerVehicleSeeder: %w", err)
		}
		if err := cmsSeeder(tx); err != nil {
			return fmt.Errorf("CMSSeeder: %w", err)
		}
		if err := settingSeeder(tx, branch); err != nil {
			return fmt.Errorf("SettingSeeder: %w", err)
		}
		return nil
	})
}

func branchSeeder(tx *gorm.DB) (model.Branch, error) {
	row := model.Branch{Base: model.Base{ID: stable("branch-pusat")}, Code: "PUSAT", Name: "Bengkel Maju Motor", Address: "Jl. Raya Otomotif No. 88, Jakarta", Phone: "021-555-0101", Timezone: "Asia/Jakarta", Currency: "IDR", IsActive: true}
	err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "code"}}, DoUpdates: clause.AssignmentColumns([]string{"name", "address", "phone", "timezone", "currency", "is_active", "updated_at"})}).Create(&row).Error
	return row, err
}

func rolePermissionSeeder(tx *gorm.DB) error {
	permissions := []string{"dashboard.read", "branch.manage", "customer.read", "customer.write", "vehicle.read", "vehicle.write", "product.read", "product.write", "inventory.read", "inventory.adjust", "work_order.read", "work_order.write", "sale.read", "sale.create", "payment.read", "payment.manage", "accounting.read", "accounting.write", "report.read", "cms.manage", "user.manage", "role.manage", "audit.read", "settings.manage"}
	for _, code := range permissions {
		row := model.Permission{Base: model.Base{ID: stable("permission-" + code)}, Code: code, Name: code}
		if err := upsert(tx, &row, []string{"name", "updated_at"}); err != nil {
			return err
		}
	}
	roles := map[string][]string{
		"owner":    permissions,
		"manager":  {"dashboard.read", "customer.read", "customer.write", "vehicle.read", "vehicle.write", "product.read", "product.write", "inventory.read", "inventory.adjust", "work_order.read", "work_order.write", "sale.read", "sale.create", "payment.read", "payment.manage", "accounting.read", "accounting.write", "report.read", "audit.read"},
		"cashier":  {"dashboard.read", "customer.read", "customer.write", "vehicle.read", "vehicle.write", "product.read", "inventory.read", "work_order.read", "work_order.write", "sale.read", "sale.create", "payment.read", "payment.manage"},
		"mechanic": {"dashboard.read", "customer.read", "vehicle.read", "product.read", "inventory.read", "work_order.read", "work_order.write"},
	}
	for roleCode, grants := range roles {
		role := model.Role{Base: model.Base{ID: stable("role-" + roleCode)}, Code: roleCode, Name: title(roleCode), Description: title(roleCode) + " access"}
		if err := upsert(tx, &role, []string{"name", "description", "updated_at"}); err != nil {
			return err
		}
		for _, permission := range grants {
			if err := tx.Exec(`INSERT INTO role_permissions(role_id,permission_id,created_at) SELECT ?,id,now() FROM permissions WHERE code=? ON CONFLICT DO NOTHING`, role.ID, permission).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func userSeeder(tx *gorm.DB, branch model.Branch) ([]model.User, error) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("Bengkel123!"), bcrypt.DefaultCost)
	specs := []struct{ Name, Email, Role string }{{"Owner Bengkel", "owner@bengkel.local", "owner"}, {"Manager Operasional", "manager@bengkel.local", "manager"}, {"Kasir Demo", "kasir@bengkel.local", "cashier"}, {"Montir Andi", "montir@bengkel.local", "mechanic"}}
	rows := make([]model.User, 0, len(specs))
	for _, spec := range specs {
		row := model.User{Base: model.Base{ID: stable("user-" + spec.Role)}, BranchID: &branch.ID, Name: spec.Name, Email: spec.Email, PasswordHash: string(hash), IsActive: true}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&row).Error; err != nil {
			return nil, err
		}
		if err := tx.Where("email=?", spec.Email).First(&row).Error; err != nil {
			return nil, err
		}
		if err := tx.Exec(`INSERT INTO user_roles(user_id,role_id,branch_id,created_at) SELECT ?,id,?,now() FROM roles WHERE code=? ON CONFLICT DO NOTHING`, row.ID, branch.ID, spec.Role).Error; err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func accountSeeder(tx *gorm.DB, branch model.Branch) error {
	accounts := []struct{ Code, Name, Type string }{{"1101", "Kas dan Bank", "asset"}, {"1201", "Persediaan Suku Cadang", "asset"}, {"1301", "Piutang Usaha", "asset"}, {"2101", "Utang Usaha", "liability"}, {"3101", "Modal Pemilik", "equity"}, {"4101", "Pendapatan Penjualan dan Jasa", "revenue"}, {"4201", "Pemulihan Biaya Payment Gateway", "revenue"}, {"5101", "Harga Pokok Penjualan", "expense"}, {"5201", "Beban Operasional", "expense"}, {"5202", "Beban Payment Gateway", "expense"}}
	for _, a := range accounts {
		row := model.Account{Base: model.Base{ID: stable("account-" + a.Code)}, BranchID: &branch.ID, Code: a.Code, Name: a.Name, Type: a.Type, IsActive: true}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func productSeeder(tx *gorm.DB, branch model.Branch) ([]model.Product, error) {
	specs := []model.Product{
		{Base: model.Base{ID: stable("product-oli")}, BranchID: &branch.ID, SKU: "OLI-10W40", Name: "Oli Mesin 10W-40", Type: "part", Unit: "botol", CostPrice: 45000, SellingPrice: 65000, MinStock: 10, IsActive: true},
		{Base: model.Base{ID: stable("product-busi")}, BranchID: &branch.ID, SKU: "BUSI-STD", Name: "Busi Standar", Type: "part", Unit: "pcs", CostPrice: 18000, SellingPrice: 30000, MinStock: 8, IsActive: true},
		{Base: model.Base{ID: stable("product-kampas")}, BranchID: &branch.ID, SKU: "KAMPAS-DEPAN", Name: "Kampas Rem Depan", Type: "part", Unit: "set", CostPrice: 55000, SellingPrice: 85000, MinStock: 5, IsActive: true},
		{Base: model.Base{ID: stable("service-tuneup")}, BranchID: &branch.ID, SKU: "JASA-TUNEUP", Name: "Jasa Tune Up", Type: "service", Unit: "jasa", CostPrice: 0, SellingPrice: 80000, MinStock: 0, IsActive: true},
		{Base: model.Base{ID: stable("service-oli")}, BranchID: &branch.ID, SKU: "JASA-GANTI-OLI", Name: "Jasa Ganti Oli", Type: "service", Unit: "jasa", CostPrice: 0, SellingPrice: 20000, MinStock: 0, IsActive: true},
	}
	for i := range specs {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&specs[i]).Error; err != nil {
			return nil, err
		}
	}
	return specs, nil
}

func inventorySeeder(tx *gorm.DB, branch model.Branch, user model.User, products []model.Product) error {
	for _, p := range products {
		if p.Type != "part" {
			continue
		}
		balance := model.InventoryBalance{BranchID: branch.ID, ProductID: p.ID, Quantity: 25}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&balance)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			movement := model.InventoryMovement{BranchID: branch.ID, ProductID: p.ID, ReferenceType: "opening_balance", Direction: "in", Quantity: 25, UnitCost: p.CostPrice, Notes: "Saldo awal DatabaseSeeder", CreatedBy: user.ID}
			if err := tx.Create(&movement).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func openingJournalSeeder(tx *gorm.DB, branch model.Branch, user model.User, products []model.Product) error {
	var inventoryValue int64
	for _, product := range products {
		if product.Type == "part" {
			inventoryValue += product.CostPrice * 25
		}
	}
	const openingCash int64 = 25_000_000
	var cash, inventory, equity model.Account
	if err := tx.Where("branch_id=? AND code='1101'", branch.ID).First(&cash).Error; err != nil {
		return err
	}
	if err := tx.Where("branch_id=? AND code='1201'", branch.ID).First(&inventory).Error; err != nil {
		return err
	}
	if err := tx.Where("branch_id=? AND code='3101'", branch.ID).First(&equity).Error; err != nil {
		return err
	}
	entry := model.JournalEntry{
		Base: model.Base{ID: stable("journal-opening-" + branch.Code)}, BranchID: branch.ID,
		Number: "OPENING-001", EntryDate: time.Date(time.Now().Year(), 1, 1, 0, 0, 0, 0, time.UTC),
		Description: "Saldo awal bengkel", ReferenceType: "opening_balance", Status: "draft", CreatedBy: user.ID,
	}
	result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&entry)
	if result.Error != nil || result.RowsAffected == 0 {
		return result.Error
	}
	lines := []model.JournalLine{
		{Base: model.Base{ID: stable("journal-opening-cash-" + branch.Code)}, JournalEntryID: entry.ID, AccountID: cash.ID, Description: "Saldo awal kas", Debit: openingCash},
		{Base: model.Base{ID: stable("journal-opening-inventory-" + branch.Code)}, JournalEntryID: entry.ID, AccountID: inventory.ID, Description: "Saldo awal persediaan", Debit: inventoryValue},
		{Base: model.Base{ID: stable("journal-opening-equity-" + branch.Code)}, JournalEntryID: entry.ID, AccountID: equity.ID, Description: "Modal awal pemilik", Credit: openingCash + inventoryValue},
	}
	if err := tx.Create(&lines).Error; err != nil {
		return err
	}
	return tx.Model(&entry).Update("status", "posted").Error
}

func customerVehicleSeeder(tx *gorm.DB, branch model.Branch) error {
	customer := model.Customer{Base: model.Base{ID: stable("customer-budi")}, BranchID: branch.ID, Code: "CUST-0001", Name: "Budi Santoso", Phone: "081234567890", Address: "Jakarta"}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&customer).Error; err != nil {
		return err
	}
	vehicle := model.Vehicle{Base: model.Base{ID: stable("vehicle-budi")}, BranchID: branch.ID, CustomerID: &customer.ID, Identifier: "B 1234 XYZ", PlateNumber: "B 1234 XYZ", Brand: "Honda", Model: "Vario 150", Year: 2021, Color: "Hitam", Odometer: 24500}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&vehicle).Error
}

func cmsSeeder(tx *gorm.DB) error {
	content := map[string]any{"hero": map[string]any{"eyebrow": "Servis motor tanpa drama", "title": "Motor sehat, perjalanan tenang.", "description": "Perawatan transparan, suku cadang terjamin, dan riwayat servis yang selalu tercatat.", "primary_cta": "Buat Janji Servis"}, "features": []map[string]string{{"title": "Pemeriksaan menyeluruh", "description": "Diagnosis jelas sebelum pengerjaan dimulai."}, {"title": "Harga transparan", "description": "Setiap barang dan jasa tercatat rinci."}, {"title": "Mekanik berpengalaman", "description": "Ditangani tim yang paham motor Anda."}}}
	row := model.CMSPage{Base: model.Base{ID: stable("cms-home")}, Slug: "home", Title: "Beranda", MetaTitle: "Bengkel Maju Motor — Servis Motor Terpercaya", MetaDescription: "Servis dan perawatan motor dengan mekanik berpengalaman, harga transparan, dan suku cadang berkualitas.", Content: content, Status: "published"}
	now := time.Now()
	row.PublishedAt = &now
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"title", "meta_title", "meta_description", "content", "status", "published_at", "updated_at"})}).Create(&row).Error
}

func settingSeeder(tx *gorm.DB, branch model.Branch) error {
	settings := []model.Setting{
		{Base: model.Base{ID: stable("setting-midtrans-channels")}, BranchID: &branch.ID, Key: "payment.midtrans.channels", Value: json.RawMessage(`{"automatic_fee":true,"channels":[{"payment_type":"bca_va","label":"BCA Virtual Account","enabled":true,"customer_percentage":100,"fee_percentage":0,"fixed_fee":4000,"tax_percentage":11},{"payment_type":"bni_va","label":"BNI Virtual Account","enabled":true,"customer_percentage":100,"fee_percentage":0,"fixed_fee":4000,"tax_percentage":11},{"payment_type":"bri_va","label":"BRI Virtual Account","enabled":true,"customer_percentage":100,"fee_percentage":0,"fixed_fee":4000,"tax_percentage":11},{"payment_type":"permata_va","label":"Permata Virtual Account","enabled":true,"customer_percentage":100,"fee_percentage":0,"fixed_fee":4000,"tax_percentage":11},{"payment_type":"echannel","label":"Mandiri Bill Payment","enabled":true,"customer_percentage":100,"fee_percentage":0,"fixed_fee":4000,"tax_percentage":11},{"payment_type":"gopay","label":"GoPay","enabled":true,"customer_percentage":100,"fee_percentage":2,"fixed_fee":0,"tax_percentage":11},{"payment_type":"qris","acquirer":"gopay","label":"QRIS","enabled":true,"customer_percentage":100,"fee_percentage":0.7,"fixed_fee":0,"tax_percentage":0},{"payment_type":"shopeepay","label":"ShopeePay","enabled":true,"customer_percentage":100,"fee_percentage":2,"fixed_fee":0,"tax_percentage":11},{"payment_type":"credit_card","label":"Kartu kredit","enabled":true,"customer_percentage":100,"fee_percentage":2.9,"fixed_fee":2000,"tax_percentage":11},{"payment_type":"indomaret","label":"Indomaret","enabled":true,"customer_percentage":100,"fee_percentage":0,"fixed_fee":1000,"tax_percentage":0},{"payment_type":"alfamart","label":"Alfamart / Alfamidi / DAN+DAN","enabled":true,"customer_percentage":100,"fee_percentage":0,"fixed_fee":5000,"tax_percentage":0},{"payment_type":"akulaku","label":"Akulaku PayLater","enabled":true,"customer_percentage":100,"fee_percentage":1.7,"fixed_fee":0,"tax_percentage":11}]}`)},
		{Base: model.Base{ID: stable("setting-tax")}, BranchID: &branch.ID, Key: "transaction.tax", Value: json.RawMessage(`{"enabled":false,"percentage":11}`)},
		{Base: model.Base{ID: stable("setting-business")}, BranchID: &branch.ID, Key: "business.profile", Value: json.RawMessage(`{"name":"Bengkel Maju Motor","whatsapp":"6281234567890","opening_hours":"Senin–Sabtu, 08.00–17.00"}`), IsPublic: true},
	}
	for i := range settings {
		var existing model.Setting
		result := tx.Where("branch_id=? AND key=?", branch.ID, settings[i].Key).Limit(1).Find(&existing)
		err := result.Error
		if err == nil && result.RowsAffected == 0 {
			err = tx.Create(&settings[i]).Error
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func upsert(tx *gorm.DB, value any, columns []string) error {
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "code"}},
		DoUpdates: clause.AssignmentColumns(columns),
	}).Create(value).Error
}
func stable(value string) uuid.UUID { return uuid.NewSHA1(uuid.NameSpaceOID, []byte("bengkel:"+value)) }
func title(value string) string {
	if value == "" {
		return value
	}
	return string(value[0]-32) + value[1:]
}
