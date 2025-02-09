package models

type Recipe struct {
    FoodID         int         `json:"foodId"`
    Instructions   string      `json:"instructions"`
    URL           string      `json:"url"`
    YieldQuantity float64     `json:"yieldQuantity"`
    YieldUnit     string      `json:"yieldUnit"`
    Ingredients   []Ingredient `json:"ingredients"`
}

type Ingredient struct {
    FoodID   int     `json:"foodId"`
    Quantity float64 `json:"quantity"`
    Unit     string  `json:"unit"`
}