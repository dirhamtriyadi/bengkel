package handler

import (
	"encoding/json"
	"errors"
	"math"
	"strings"

	"bengkel/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const midtransChannelsSettingKey = "payment.midtrans.channels"

type midtransFeeSettings struct {
	AutomaticFee bool                      `json:"automatic_fee"`
	Channels     []midtransChannelSettings `json:"channels"`
}

type midtransChannelSettings struct {
	PaymentType        string  `json:"payment_type"`
	Acquirer           string  `json:"acquirer,omitempty"`
	Label              string  `json:"label"`
	Enabled            bool    `json:"enabled"`
	CustomerPercentage float64 `json:"customer_percentage"`
	FeePercentage      float64 `json:"fee_percentage"`
	FixedFee           int64   `json:"fixed_fee"`
	TaxPercentage      float64 `json:"tax_percentage"`
}

func defaultMidtransFeeSettings() midtransFeeSettings {
	return midtransFeeSettings{
		// Automatic Fee Imposition is an account-level Midtrans capability.
		// Keep it opt-in so a merchant without that capability can still pay.
		AutomaticFee: false,
		Channels: []midtransChannelSettings{
			{PaymentType: "bca_va", Label: "BCA Virtual Account", Enabled: true, CustomerPercentage: 100, FixedFee: 4000, TaxPercentage: 11},
			{PaymentType: "bni_va", Label: "BNI Virtual Account", Enabled: true, CustomerPercentage: 100, FixedFee: 4000, TaxPercentage: 11},
			{PaymentType: "bri_va", Label: "BRI Virtual Account", Enabled: true, CustomerPercentage: 100, FixedFee: 4000, TaxPercentage: 11},
			{PaymentType: "permata_va", Label: "Permata Virtual Account", Enabled: true, CustomerPercentage: 100, FixedFee: 4000, TaxPercentage: 11},
			{PaymentType: "echannel", Label: "Mandiri Bill Payment", Enabled: true, CustomerPercentage: 100, FixedFee: 4000, TaxPercentage: 11},
			{PaymentType: "gopay", Label: "GoPay", Enabled: true, CustomerPercentage: 100, FeePercentage: 2, TaxPercentage: 11},
			{PaymentType: "qris", Acquirer: "gopay", Label: "QRIS", Enabled: true, CustomerPercentage: 100, FeePercentage: 0.7},
			{PaymentType: "shopeepay", Label: "ShopeePay", Enabled: true, CustomerPercentage: 100, FeePercentage: 2, TaxPercentage: 11},
			{PaymentType: "credit_card", Label: "Kartu kredit", Enabled: true, CustomerPercentage: 100, FeePercentage: 2.9, FixedFee: 2000, TaxPercentage: 11},
			{PaymentType: "indomaret", Label: "Indomaret", Enabled: true, CustomerPercentage: 100, FixedFee: 1000},
			{PaymentType: "alfamart", Label: "Alfamart / Alfamidi / DAN+DAN", Enabled: true, CustomerPercentage: 100, FixedFee: 5000},
			{PaymentType: "akulaku", Label: "Akulaku PayLater", Enabled: true, CustomerPercentage: 100, FeePercentage: 1.7, TaxPercentage: 11},
		},
	}
}

func (h *Handler) midtransFeeSettings(branchID uuid.UUID) (midtransFeeSettings, error) {
	var setting model.Setting
	err := h.DB.Where("branch_id=? AND key=?", branchID, midtransChannelsSettingKey).First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultMidtransFeeSettings(), nil
	}
	if err != nil {
		return midtransFeeSettings{}, err
	}
	return parseMidtransFeeSettings(setting.Value)
}

func parseMidtransFeeSettings(raw json.RawMessage) (midtransFeeSettings, error) {
	var settings midtransFeeSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return settings, errors.New("konfigurasi channel Midtrans bukan JSON yang valid")
	}
	if len(settings.Channels) == 0 {
		return settings, errors.New("minimal satu channel Midtrans harus dikonfigurasi")
	}
	seen := map[string]struct{}{}
	enabled := 0
	for index := range settings.Channels {
		channel := &settings.Channels[index]
		channel.PaymentType = strings.TrimSpace(channel.PaymentType)
		channel.Acquirer = strings.TrimSpace(channel.Acquirer)
		channel.Label = strings.TrimSpace(channel.Label)
		if channel.PaymentType == "" || channel.Label == "" {
			return settings, errors.New("payment_type dan label channel Midtrans wajib diisi")
		}
		if _, exists := seen[channel.PaymentType]; exists {
			return settings, errors.New("payment_type channel Midtrans tidak boleh duplikat")
		}
		seen[channel.PaymentType] = struct{}{}
		if channel.CustomerPercentage < 0 || channel.CustomerPercentage > 100 {
			return settings, errors.New("porsi biaya pelanggan harus antara 0 sampai 100 persen")
		}
		if settings.AutomaticFee && channel.Enabled && channel.PaymentType == "qris" && channel.Acquirer == "" {
			return settings, errors.New("channel QRIS wajib memiliki acquirer untuk fee otomatis Midtrans")
		}
		if channel.FeePercentage < 0 || channel.FeePercentage > 100 || channel.FixedFee < 0 || channel.TaxPercentage < 0 || channel.TaxPercentage > 100 {
			return settings, errors.New("referensi biaya channel Midtrans tidak valid")
		}
		if channel.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		return settings, errors.New("minimal satu channel Midtrans harus diaktifkan")
	}
	return settings, nil
}

func (settings midtransFeeSettings) enabledChannels() []midtransChannelSettings {
	channels := make([]midtransChannelSettings, 0, len(settings.Channels))
	for _, channel := range settings.Channels {
		if channel.Enabled {
			channels = append(channels, channel)
		}
	}
	return channels
}

func (settings midtransFeeSettings) feeBearer() string {
	if !settings.AutomaticFee {
		return "merchant"
	}
	hasMerchantShare := false
	hasCustomerShare := false
	for _, channel := range settings.enabledChannels() {
		if channel.CustomerPercentage < 100 {
			hasMerchantShare = true
		}
		if channel.CustomerPercentage > 0 {
			hasCustomerShare = true
		}
	}
	if hasCustomerShare && !hasMerchantShare {
		return "customer"
	}
	if hasMerchantShare && !hasCustomerShare {
		return "merchant"
	}
	return "split"
}

func (settings midtransFeeSettings) channel(paymentType string) (midtransChannelSettings, bool) {
	for _, channel := range settings.Channels {
		if strings.EqualFold(channel.PaymentType, paymentType) {
			return channel, true
		}
	}
	return midtransChannelSettings{}, false
}

func estimateMidtransProviderFee(amount int64, channel midtransChannelSettings) int64 {
	baseFee := float64(channel.FixedFee) + float64(amount)*channel.FeePercentage/100
	total := baseFee * (1 + channel.TaxPercentage/100)
	return int64(math.Ceil(total - 1e-9))
}

func deriveMidtransProviderFee(baseAmount, customerFee int64, channel midtransChannelSettings) int64 {
	if customerFee > 0 && channel.CustomerPercentage > 0 {
		return int64(math.Ceil(float64(customerFee) * 100 / channel.CustomerPercentage))
	}
	return estimateMidtransProviderFee(baseAmount, channel)
}

func decodeMidtransFeeSnapshot(value any) (midtransFeeSettings, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return midtransFeeSettings{}, err
	}
	return parseMidtransFeeSettings(raw)
}
