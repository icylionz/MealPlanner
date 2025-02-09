package models

type Food struct {
    ID       int     `json:"id"`
    Name     string  `json:"name"`
    UnitType string  `json:"unitType"`
    BaseUnit string  `json:"baseUnit"`
    Density  float64 `json:"density,omitempty"`
    IsRecipe bool    `json:"isRecipe"`
}