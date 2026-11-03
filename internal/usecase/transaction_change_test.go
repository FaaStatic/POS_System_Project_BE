package usecase

import (
	"context"
	"errors"
	"testing"

	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type changeProductRepo struct {
	domain.ProductRepository
	products map[uuid.UUID]*domain.Products
}

func (f *changeProductRepo) GetProductsByIDs(ctx context.Context, ids []uuid.UUID, includeDeleted bool) (map[uuid.UUID]*domain.Products, error) {
	return f.products, nil
}

func TestBuildChange(t *testing.T) {
	kg, _ := enum.ParseProductUnit("kg")
	candy := &domain.Products{ID: uuid.New(), ProductName: "Permen", SellingPrice: 500, PurchasePrice: 300}
	rice := &domain.Products{ID: uuid.New(), ProductName: "Beras", Unit: kg, SellingPrice: 14000, PurchasePrice: 12000}
	digital := &domain.Products{ID: uuid.New(), ProductName: "Pulsa", ProductType: enum.Digital, SellingPrice: 5000}
	uc := &transactionUsecase{
		productRepo: &changeProductRepo{products: map[uuid.UUID]*domain.Products{candy.ID: candy, rice.ID: rice, digital.ID: digital}},
		log:         zap.NewNop(),
	}
	cash, _ := enum.ParseMoneyPayment("tunai")
	transfer, _ := enum.ParseMoneyPayment("transfer")

	paid := func(n int64) *int64 { return &n }
	ref := func(p *domain.Products) *string { s := p.ID.String(); return &s }

	tests := []struct {
		name       string
		payment    enum.MoneyPayment
		paid       *int64
		change     []requestdto.ChangeLineRequest
		total      int64
		wantDue    int64
		wantLines  int
		wantErr    bool
		wantAmount []int64
	}{
		{name: "no amount_paid records nothing", payment: cash, total: 40000},
		{name: "cash only", payment: cash, paid: paid(50000), total: 40000, wantDue: 10000, wantLines: 1, wantAmount: []int64{10000},
			change: []requestdto.ChangeLineRequest{{Kind: "tunai", Amount: 10000}}},
		{name: "mixed cash, candy and half a kg of rice", payment: cash, paid: paid(52000), total: 40000, wantDue: 12000, wantLines: 3, wantAmount: []int64{3000, 2000, 7000},
			change: []requestdto.ChangeLineRequest{
				{Kind: "tunai", Amount: 3000},
				{Kind: "permen", ProductId: ref(candy), Qty: 4},
				{Kind: "produk_lain", ProductId: ref(rice), Qty: 0.5},
			}},
		{name: "four decimals refused", payment: cash, paid: paid(44000), total: 40000, wantErr: true,
			change: []requestdto.ChangeLineRequest{{Kind: "produk_lain", ProductId: ref(rice), Qty: 0.2857}}},
		{name: "exact payment needs no change", payment: cash, paid: paid(40000), total: 40000},
		{name: "underpaid", payment: cash, paid: paid(30000), total: 40000, wantErr: true},
		{name: "lines do not add up", payment: cash, paid: paid(50000), total: 40000, wantErr: true,
			change: []requestdto.ChangeLineRequest{{Kind: "tunai", Amount: 9000}}},
		{name: "unknown kind refused (offline sync has no dto validation)", payment: cash, paid: paid(50000), total: 40000, wantErr: true,
			change: []requestdto.ChangeLineRequest{{Kind: "x", ProductId: ref(candy), Qty: 1, Amount: 500}, {Kind: "tunai", Amount: 9500}}},
		{name: "change due but no lines", payment: cash, paid: paid(50000), total: 40000, wantErr: true},
		{name: "change without amount_paid", payment: cash, total: 40000, wantErr: true,
			change: []requestdto.ChangeLineRequest{{Kind: "tunai", Amount: 1000}}},
		{name: "only tunai may carry change", payment: transfer, paid: paid(50000), total: 40000, wantErr: true},
		{name: "client price mismatch", payment: cash, paid: paid(41000), total: 40000, wantErr: true,
			change: []requestdto.ChangeLineRequest{{Kind: "permen", ProductId: ref(candy), Qty: 2, Amount: 999}}},
		{name: "pcs cannot be fractional", payment: cash, paid: paid(40750), total: 40000, wantErr: true,
			change: []requestdto.ChangeLineRequest{{Kind: "permen", ProductId: ref(candy), Qty: 1.5}}},
		{name: "digital product refused", payment: cash, paid: paid(45000), total: 40000, wantErr: true,
			change: []requestdto.ChangeLineRequest{{Kind: "produk_lain", ProductId: ref(digital), Qty: 1}}},
		{name: "product line needs a product", payment: cash, paid: paid(41000), total: 40000, wantErr: true,
			change: []requestdto.ChangeLineRequest{{Kind: "permen", Qty: 2}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			due, lines, err := uc.buildChange(context.Background(), &requestdto.AddTransactionRequest{AmountPaid: tt.paid, Change: tt.change}, tt.payment, tt.total)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("err = %v, want a validation error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if due != tt.wantDue || len(lines) != tt.wantLines {
				t.Fatalf("due/lines = %d/%d, want %d/%d", due, len(lines), tt.wantDue, tt.wantLines)
			}
			for i, want := range tt.wantAmount {
				if lines[i].Amount != want {
					t.Errorf("line %d amount = %d, want %d", i, lines[i].Amount, want)
				}
			}
		})
	}
}
