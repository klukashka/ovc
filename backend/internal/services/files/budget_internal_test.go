package files

import (
	"image"
	"image/color"
	"testing"

	"ovc/backend/internal/domain"
)

func TestDecodeBudgetAdmitsOneDecodePerUserWithinItsBytes(t *testing.T) {
	budget := newDecodeBudget(100)
	first, second := domain.UserID{1}, domain.UserID{2}
	release, admitted := budget.take(first, 60)
	if !admitted {
		t.Fatal("the first decode was refused")
	}

	_, sameUser := budget.take(first, 10)
	_, overBudget := budget.take(second, 41)
	releaseFit, withinBudget := budget.take(second, 40)
	release()
	releaseFit()
	_, afterRelease := budget.take(first, 100)

	if sameUser || overBudget || !withinBudget || !afterRelease {
		t.Fatalf("same user %v, over budget %v, within budget %v, after release %v", sameUser, overBudget, withinBudget, afterRelease)
	}
}

func TestDecodeCostRefusesDeepAndCMYKImagesAndWeighsJPEGHigher(t *testing.T) {
	side := domain.AvatarMaxSide
	cases := []struct {
		model     color.Model
		format    string
		cost      int64
		decodable bool
	}{
		{model: color.NRGBAModel, format: "png", cost: int64(side) * int64(side) * imageBytesPerPixel, decodable: true},
		{model: color.YCbCrModel, format: "jpeg", cost: int64(side) * int64(side) * jpegBytesPerPixel, decodable: true},
		{model: color.NRGBAModel, format: "webp", cost: 200*200*webpBytesPerPixel + webpTreeAllowance, decodable: true},
		{model: color.NRGBA64Model, format: "png", decodable: false},
		{model: color.CMYKModel, format: "jpeg", decodable: false},
	}

	for _, testCase := range cases {
		width := side
		if testCase.format == "webp" {
			width = 200
		}
		cost, decodable := decodeCost(image.Config{ColorModel: testCase.model, Width: width, Height: width}, testCase.format)

		if decodable != testCase.decodable || (decodable && cost != testCase.cost) {
			t.Errorf("%s %v: cost %d, decodable %v", testCase.format, testCase.model, cost, decodable)
		}
	}
	for format, worst := range map[string]int64{
		"jpeg": int64(side) * int64(side) * jpegBytesPerPixel,
		"webp": max(int64(side)*int64(side)*imageBytesPerPixel, int64(side)*int64(side)*webpBytesPerPixel+webpTreeAllowance),
	} {
		if worst > decodeBudgetBytes {
			t.Fatalf("the largest %s needs %d bytes, more than the whole budget", format, worst)
		}
	}
}
