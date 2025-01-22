document.addEventListener("alpine:init", () => {
  Alpine.data("foodManager", () => ({
    // UI State
    showFoodModal: false,
    editingFood: null,
    searchQuery: "",
    typeFilter: "all",
    formData: null,
    showDeleteModal: false,

    // Initialize
    init() {
      this.initFormData();
    },

    // Form Data Management
    initFormData() {
      this.formData = {
        id: null,
        name: "",
        unitType: "mass",
        baseUnit: "grams",
        density: null,
        isRecipe: false,
        recipe: {
          ingredients: [],
          instructions: "",
          url: "",
          yield: {
            quantity: 0,
            unit: "servings",
          },
        },
      };
    },

    // Food Management Methods
    getFoodById(id) {
      return this.$store.mealPlanner.foods.find((f) => f.id === String(id));
    },

    validateRecipeDepth(foodId, visited = new Set()) {
      if (visited.size >= 15) return false;

      const food = this.getFoodById(foodId);
      if (!food?.isRecipe) return true;

      visited.add(foodId);

      for (const ing of food.recipe.ingredients) {
        if (visited.has(ing.foodId)) return false;
        if (!this.validateRecipeDepth(ing.foodId, new Set(visited))) {
          return false;
        }
      }

      return true;
    },

    isFoodUsedInRecipes(foodId) {
      return this.$store.mealPlanner.foods.some(
        (food) =>
          food.isRecipe &&
          food.recipe.ingredients.some((ing) => ing.foodId === String(foodId)),
      );
    },

    // Computed Properties
    get filteredFoods() {
      return this.$store.mealPlanner.foods.filter((food) => {
        const matchesSearch = food.name
          .toLowerCase()
          .includes(this.searchQuery.toLowerCase());
        const matchesType =
          this.typeFilter === "all" ||
          (this.typeFilter === "recipe" && food.isRecipe) ||
          (this.typeFilter === "basic" && !food.isRecipe);
        return matchesSearch && matchesType;
      });
    },

    get availableIngredients() {
      const editingId = this.editingFood?.id;
      // Filter out current food and check for circular dependencies if it's a recipe
      return this.$store.mealPlanner.foods.filter((food) => {
        if (editingId && food.id === editingId) return false;
        if (!this.formData.isRecipe) return true;
        return this.validateRecipeDepth(
          food.id,
          new Set(editingId ? [editingId] : []),
        );
      });
    },

    // CRUD Operations
    async addFood(foodData) {
      const newFood = {
        ...foodData,
        id: String(Date.now()),
      };

      if (foodData.isRecipe) {
        const validation = this.validateRecipeDepth(newFood.id);
        if (!validation) {
          throw new Error(
            "Recipe would create a circular dependency or exceed maximum depth",
          );
        }
      }

      this.$store.mealPlanner.foods.push(newFood);
      return newFood;
    },

    async updateFood(id, foodData) {
      const index = this.$store.mealPlanner.foods.findIndex(
        (f) => f.id === String(id),
      );
      if (index === -1) throw new Error("Food not found");

      const updatedFood = {
        ...this.$store.mealPlanner.foods[index],
        ...foodData,
        id: String(id),
      };

      if (foodData.isRecipe) {
        const validation = this.validateRecipeDepth(updatedFood.id);
        if (!validation) {
          throw new Error(
            "Recipe would create a circular dependency or exceed maximum depth",
          );
        }
      }

      this.$store.mealPlanner.foods[index] = updatedFood;
      return updatedFood;
    },

    // Recipe Management
    addIngredient() {
      this.formData.recipe.ingredients.push({
        foodId: "",
        quantity: 0,
        unit: "grams",
      });
    },

    removeIngredient(index) {
      this.formData.recipe.ingredients.splice(index, 1);
    },

    // Unit Handling
    getUnitsForType(type) {
      return Alpine.store("units").getUnitsForType(type);
    },

    getCompatibleUnits(unit, foodId) {
      const food = this.getFoodById(foodId);
      if (!food) return this.getUnitsForType("mass");
      return this.getUnitsForType(food.unitType);
    },

    formatQuantity(value, unit) {
      return Alpine.store("units").formatQuantity(value, unit);
    },

    // Modal Management
    openNewFoodModal() {
      this.editingFood = null;
      this.initFormData();
      this.showFoodModal = true;
    },

    openFoodModal(food) {
      this.editingFood = food;
      // Initialize with a fresh form first
      this.initFormData();
      // Then carefully merge the existing food data
      if (food) {
        this.formData = {
          ...this.formData,
          ...food,
          recipe: food.isRecipe
            ? {
                ingredients: Array.isArray(food.recipe.ingredients)
                  ? food.recipe.ingredients.map((ing) => ({ ...ing }))
                  : [],
                instructions: food.recipe.instructions || "",
                url: food.recipe.url || "",
                yield: {
                  quantity: food.recipe.yield?.quantity || 0,
                  unit: food.recipe.yield?.unit || "servings",
                },
              }
            : this.formData.recipe,
        };
      }
      this.showFoodModal = true;
    },

    // Form Submission
    validateForm() {
      if (!this.formData.name.trim()) return false;

      if (this.formData.isRecipe) {
        if (
          !this.formData.recipe.yield.quantity ||
          !this.formData.recipe.yield.unit
        )
          return false;
        if (!this.formData.recipe.ingredients.length) return false;

        // Validate ingredients
        for (const ing of this.formData.recipe.ingredients) {
          if (!ing.foodId || !ing.quantity || !ing.unit) return false;
        }
      }

      return true;
    },

    async saveFood() {
      if (!this.validateForm()) {
        alert("Please fill in all required fields");
        return;
      }

      try {
        if (this.editingFood) {
          await this.updateFood(this.editingFood.id, this.formData);
        } else {
          await this.addFood(this.formData);
        }
        this.showFoodModal = false;
      } catch (error) {
        alert(error.message);
      }
    },

    confirmDelete(food) {
      this.showDeleteModal = true;
      window.dispatchEvent(
        new CustomEvent("confirm-delete", {
          detail: food,
        }),
      );
    },

    async deleteFood(id) {
      try {
        if (this.isFoodUsedInRecipes(id)) {
          window.dispatchEvent(
            new CustomEvent("show-alert", {
              detail: {
                message:
                  "Cannot delete food as it is used in one or more recipes",
                type: "error",
              },
            }),
          );
          return false;
        }

        this.$store.mealPlanner.foods = this.$store.mealPlanner.foods.filter(
          (f) => f.id !== String(id),
        );

        return true;
      } catch (error) {
        window.dispatchEvent(
          new CustomEvent("show-alert", {
            detail: {
              message: error.message,
              type: "error",
            },
          }),
        );
        return false;
      }
    },
    
  }));
});
