package shopping

import "testing"

func qty(v float64) *float64 { return &v }

func TestSumsSameIngredientSameUnit(t *testing.T) {
	items := Aggregate([]Line{
		{IngredientID: 1, Name: "排骨", Category: "meat", Unit: "g", Qty: qty(400)},
		{IngredientID: 1, Name: "排骨", Category: "meat", Unit: "g", Qty: qty(500)},
	})
	if len(items) != 1 || *items[0].TotalQty != 900 {
		t.Fatalf("expected one 900g entry, got %+v", items)
	}
}

func TestDifferentUnitsStaySeparate(t *testing.T) {
	items := Aggregate([]Line{
		{IngredientID: 2, Name: "鸡蛋", Category: "dairy", Unit: "个", Qty: qty(4)},
		{IngredientID: 2, Name: "鸡蛋", Category: "dairy", Unit: "g", Qty: qty(100)},
	})
	if len(items) != 2 {
		t.Fatalf("different units must not merge, got %+v", items)
	}
}

func TestToTasteCollapsesToOne(t *testing.T) {
	items := Aggregate([]Line{
		{IngredientID: 3, Name: "盐", Category: "condiment", Qty: nil},
		{IngredientID: 3, Name: "盐", Category: "condiment", Qty: nil},
		{IngredientID: 3, Name: "盐", Category: "condiment", Qty: nil},
	})
	if len(items) != 1 || items[0].TotalQty != nil || items[0].Unit != "适量" {
		t.Fatalf("expected single 适量 entry, got %+v", items)
	}
}

func TestToTasteDroppedWhenQuantifiedExists(t *testing.T) {
	items := Aggregate([]Line{
		{IngredientID: 4, Name: "白糖", Category: "condiment", Unit: "g", Qty: qty(30)},
		{IngredientID: 4, Name: "白糖", Category: "condiment", Qty: nil},
	})
	if len(items) != 1 || items[0].TotalQty == nil || *items[0].TotalQty != 30 {
		t.Fatalf("适量 should be dropped when a quantified entry exists, got %+v", items)
	}
}

func TestMarketWalkOrdering(t *testing.T) {
	items := Aggregate([]Line{
		{IngredientID: 5, Name: "生抽", Category: "condiment", Unit: "ml", Qty: qty(10)},
		{IngredientID: 6, Name: "菜心", Category: "vegetable", Unit: "g", Qty: qty(400)},
		{IngredientID: 7, Name: "排骨", Category: "meat", Unit: "g", Qty: qty(400)},
		{IngredientID: 8, Name: "鲈鱼", Category: "seafood", Unit: "条", Qty: qty(1)},
	})
	want := []string{"排骨", "鲈鱼", "菜心", "生抽"}
	for i, n := range want {
		if items[i].Name != n {
			t.Fatalf("position %d: want %s got %s (full: %+v)", i, n, items[i].Name, items)
		}
	}
}
