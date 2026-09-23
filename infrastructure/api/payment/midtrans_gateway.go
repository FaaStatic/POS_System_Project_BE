package payment

import (
	"context"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"

	"shop_project_be/internal/domain"

	"github.com/midtrans/midtrans-go"
	"github.com/midtrans/midtrans-go/coreapi"
)

type midtransGateway struct {
	client    coreapi.Client
	serverKey string
}

func NewMidtransGateway(serverKey, environment string) domain.PaymentGateway {
	env := midtrans.Sandbox
	if strings.EqualFold(environment, "production") {
		env = midtrans.Production
	}
	var client coreapi.Client
	client.New(serverKey, env)
	return &midtransGateway{client: client, serverKey: serverKey}
}

func (g *midtransGateway) ChargeQris(_ context.Context, in domain.GatewayChargeInput) (*domain.GatewayChargeResult, error) {
	req := &coreapi.ChargeReq{
		PaymentType: coreapi.PaymentTypeQris,
		TransactionDetails: midtrans.TransactionDetails{
			OrderID:  in.OrderID,
			GrossAmt: in.GrossAmount,
		},
		Qris: &coreapi.QrisDetails{Acquirer: "gopay"},
	}
	res, mErr := g.client.ChargeTransaction(req)
	if mErr != nil {
		return nil, errors.New(mErr.GetMessage())
	}
	return mapChargeResponse(res), nil
}

func (g *midtransGateway) ChargeVA(_ context.Context, in domain.GatewayChargeInput) (*domain.GatewayChargeResult, error) {
	req := &coreapi.ChargeReq{
		TransactionDetails: midtrans.TransactionDetails{
			OrderID:  in.OrderID,
			GrossAmt: in.GrossAmount,
		},
	}
	switch strings.ToLower(in.Bank) {
	case "bca":
		req.PaymentType = coreapi.PaymentTypeBankTransfer
		req.BankTransfer = &coreapi.BankTransferDetails{Bank: midtrans.BankBca}
	case "mandiri":
		req.PaymentType = coreapi.PaymentTypeEChannel
		req.EChannel = &coreapi.EChannelDetail{
			BillInfo1: "Payment",
			BillInfo2: in.OrderID,
		}
	default:
		return nil, errors.New("unsupported va bank")
	}
	res, mErr := g.client.ChargeTransaction(req)
	if mErr != nil {
		return nil, errors.New(mErr.GetMessage())
	}
	out := mapChargeResponse(res)
	if out.Bank == "" {
		out.Bank = strings.ToLower(in.Bank)
	}
	return out, nil
}

func (g *midtransGateway) CheckStatus(_ context.Context, orderID string) (*domain.GatewayChargeResult, error) {
	res, mErr := g.client.CheckTransaction(orderID)
	if mErr != nil {
		return nil, errors.New(mErr.GetMessage())
	}
	return &domain.GatewayChargeResult{
		TransactionID:     res.TransactionID,
		OrderID:           res.OrderID,
		PaymentType:       res.PaymentType,
		TransactionStatus: res.TransactionStatus,
		FraudStatus:       res.FraudStatus,
		StatusCode:        res.StatusCode,
		ExpiryTime:        res.ExpiryTime,
	}, nil
}

func (g *midtransGateway) VerifySignature(orderID, statusCode, grossAmount, signatureKey string) bool {
	raw := orderID + statusCode + grossAmount + g.serverKey
	sum := sha512.Sum512([]byte(raw))
	expected := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(expected), []byte(strings.ToLower(signatureKey))) == 1
}

func mapChargeResponse(res *coreapi.ChargeResponse) *domain.GatewayChargeResult {
	out := &domain.GatewayChargeResult{
		TransactionID:     res.TransactionID,
		OrderID:           res.OrderID,
		PaymentType:       res.PaymentType,
		TransactionStatus: res.TransactionStatus,
		FraudStatus:       res.FraudStatus,
		StatusCode:        res.StatusCode,
		QRString:          res.QRString,
		RedirectURL:       res.RedirectURL,
		ExpiryTime:        res.ExpiryTime,
	}
	for _, a := range res.Actions {
		if a.Name == "generate-qr-code" {
			out.QRURL = a.URL
			break
		}
	}
	if len(res.VaNumbers) > 0 {
		out.VANumber = res.VaNumbers[0].VANumber
		out.Bank = res.VaNumbers[0].Bank
	}
	out.BillKey = res.BillKey
	out.BillerCode = res.BillerCode
	return out
}
