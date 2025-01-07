// main.js
function mealPlanner() {
  return {
    DateTime: luxon.DateTime,
    currentDate: luxon.DateTime.local(),
    activeTab: "calendar",
    meals: [],
    ingredients: [],
    schedules: [],
    showMealModal: false,
    showScheduleModal: false,
    showMealDetailsModal: false,
    showIngredientModal: false, 
    selectedDate: null,
    selectedMealDetails: null,
    selectedMealSchedule: null,
    selectedIngredientDetails: null,
    shoppingListStartDate: "",
    shoppingListEndDate: "",
    shoppingList: [],
    editingMealId: null,
    editingIngredientId: null,
    newSchedule: {
      time: "",
      mealId: "",
      date: null,
    },
    newMeal: {
      name: "",
      recipeUrl: "",
      ingredients: [],
      yield: { amount: 1, unit: "serving" },
    },
    newIngredient: {
      name: "",
      recipeUrl: "",
      ingredients: [],
      yield: { amount: 1, unit: "unit" },
    },

    init() {
      // Load data from localStorage
      const savedMeals = localStorage.getItem("meals");
      const savedSchedules = localStorage.getItem("schedules");
      const savedIngredients = localStorage.getItem("ingredients");

      if (savedMeals) {
        this.meals = JSON.parse(savedMeals).map((meal) => ({
          ...meal,
          id: Number(meal.id),
        }));
      }

      if (savedSchedules) {
        this.schedules = JSON.parse(savedSchedules).map((schedule) => ({
          ...schedule,
          id: Number(schedule.id),
          mealId: Number(schedule.mealId),
        }));
      }

      if (savedIngredients) {
        this.ingredients = JSON.parse(savedIngredients).map((ingredient) => ({
          ...ingredient,
          id: Number(ingredient.id),
        }));
      }
    },

    get currentMonthYear() {
      return this.currentDate.toFormat("MMMM yyyy");
    },

    get calendarDays() {
      const start = this.currentDate.startOf("month").startOf("week");
      const end = this.currentDate.endOf("month").endOf("week");
      const days = [];

      let current = start;
      while (current <= end) {
        days.push({
          date: current,
          isCurrentMonth: current.month === this.currentDate.month,
        });
        current = current.plus({ days: 1 });
      }
      return days;
    },

    previousMonth() {
      this.currentDate = this.currentDate.minus({ months: 1 });
    },

    nextMonth() {
      this.currentDate = this.currentDate.plus({ months: 1 });
    },

    openMealDetails(schedule) {
      const mealId = Number(schedule.mealId);
      const meal = this.getMealById(mealId);
      if (meal) {
        this.selectedMealDetails = meal;
        this.selectedMealSchedule = schedule;
        this.showMealDetailsModal = true;
      }
    },

    openCreateMealModal() {
      this.editingMealId = null;
      this.newMeal = {
        name: "",
        recipeUrl: "",
        ingredients: [],
        yield: {
          amount: 1,
          unit: "serving",
        },
      };
      this.showMealModal = true;
    },

    openScheduleMealModal(date) {
      this.selectedDate = date;
      this.newSchedule = {
        time: "",
        mealId: "",
        date: date.toISO(),
      };
      this.showScheduleModal = true;
    },

    loadMealForEditing(meal) {
      this.editingMealId = Number(meal.id);
      this.newMeal = {
        name: meal.name,
        recipeUrl: meal.recipeUrl,
        ingredients: [...meal.ingredients],
        yield: {
          amount: meal.yield?.amount || 1,
          unit: meal.yield?.unit || "serving",
        },
      };
      this.showMealModal = true;
      this.showMealDetailsModal = false;
    },

    wouldCreateCircularDependency(recipeId, ingredientRecipeId) {
      const visited = new Set();

      const checkDependencies = (currentRecipeId) => {
        // If we've seen this recipe before, we found a cycle
        if (visited.has(currentRecipeId)) {
          return true;
        }

        // Mark this recipe as visited
        visited.add(currentRecipeId);

        // Find the recipe
        const recipe = this.ingredients.find(
          (r) => r.id === Number(currentRecipeId)
        );
        if (!recipe) return false;

        // Check all recipe-type ingredients
        for (const ing of recipe.ingredients) {
          if (ing.type === "recipe") {
            // If this ingredient would complete a cycle back to our target recipe
            if (Number(ing.recipeId) === Number(recipeId)) {
              return true;
            }
            // Recursively check this ingredient's dependencies
            if (checkDependencies(ing.recipeId)) {
              return true;
            }
          }
        }

        // No cycles found in this branch
        visited.delete(currentRecipeId);
        return false;
      };

      return checkDependencies(ingredientRecipeId);
    },

    saveMeal() {
      if (!this.newMeal.name) {
        alert("Please enter a meal name");
        return;
      }

      for (const ing of this.newMeal.ingredients) {
        if (ing.type === "recipe" && !ing.recipeId) {
          alert("Please select a recipe for all recipe ingredients");
          return;
        }
        if (ing.type === "basic" && !ing.name) {
          alert("Please enter a name for all ingredients");
          return;
        }
      }

      // Check for circular dependencies
      for (const mealIng of this.newMeal.ingredients) {
        if (
          this.wouldCreateCircularDependency(
            this.editingMealId || Date.now(),
            Number(mealIng.mealId)
          )
        ) {
          alert(
            "Cannot add this meal as an ingredient as it would create a circular dependency"
          );
          return;
        }
      }

      if (this.editingMealId !== null) {
        const index = this.meals.findIndex((m) => m.id === this.editingMealId);
        if (index !== -1) {
          this.meals[index] = {
            ...this.meals[index],
            ...this.newMeal,
          };
        }
      } else {
        const meal = {
          id: Number(Date.now()),
          ...this.newMeal,
        };
        this.meals.push(meal);
      }

      localStorage.setItem("meals", JSON.stringify(this.meals));
      this.showMealModal = false;
    },

    saveIngredient() {
      if (!this.newIngredient.name) {
        alert("Please enter an ingredient name");
        return;
      }

      // Validate all ingredients
      for (const ing of this.newIngredient.ingredients) {
        if (ing.type === "recipe" && !ing.recipeId) {
          alert("Please select a recipe for all recipe ingredients");
          return;
        }
        if (ing.type === "basic" && !ing.name) {
          alert("Please enter a name for all ingredients");
          return;
        }
        if (!ing.amount || ing.amount <= 0) {
          alert("Please enter a valid amount for all ingredients");
          return;
        }
      }

      const ingredient = {
        id: this.editingIngredientId || Number(Date.now()),
        ...this.newIngredient,
      };

      if (this.editingIngredientId) {
        const index = this.ingredients.findIndex(
          (i) => i.id === this.editingIngredientId
        );
        if (index !== -1) {
          this.ingredients[index] = ingredient;
        }
      } else {
        this.ingredients.push(ingredient);
      }

      localStorage.setItem("ingredients", JSON.stringify(this.ingredients));
      this.showIngredientModal = false;
    },

    saveSchedule() {
      if (!this.newSchedule.time || !this.newSchedule.mealId) {
        alert("Please select both time and meal");
        return;
      }

      const schedule = {
        id: Number(Date.now()),
        date: this.selectedDate.toISO(),
        time: this.newSchedule.time,
        mealId: Number(this.newSchedule.mealId),
      };

      this.schedules.push(schedule);
      localStorage.setItem("schedules", JSON.stringify(this.schedules));
      this.showScheduleModal = false;
    },

    deleteMeal(mealId) {
      mealId = Number(mealId);
      if (
        confirm(
          "Are you sure you want to delete this meal? This will also remove all scheduled instances of this meal."
        )
      ) {
        this.meals = this.meals.filter((meal) => meal.id !== mealId);
        this.schedules = this.schedules.filter(
          (schedule) => Number(schedule.mealId) !== mealId
        );
        localStorage.setItem("meals", JSON.stringify(this.meals));
        localStorage.setItem("schedules", JSON.stringify(this.schedules));
        this.showMealDetailsModal = false;
      }
    },

    deleteScheduledMeal(scheduleId) {
      scheduleId = Number(scheduleId);
      if (confirm("Are you sure you want to remove this scheduled meal?")) {
        this.schedules = this.schedules.filter(
          (schedule) => schedule.id !== scheduleId
        );
        localStorage.setItem("schedules", JSON.stringify(this.schedules));
        this.showMealDetailsModal = false;
      }
    },
    getAvailableRecipeIngredients(currentIngredient) {
      const currentRecipeId = this.editingIngredientId;

      // Filter available recipes:
      // 1. Don't allow selecting the current recipe itself
      // 2. Don't allow recipes that would create cycles
      return this.ingredients.filter((recipe) => {
        const recipeId = Number(recipe.id);

        // Don't allow selecting the current recipe
        if (recipeId === currentRecipeId) {
          return false;
        }

        // Don't allow recipes that would create a cycle if added
        if (
          this.wouldCreateCircularDependency(
            currentRecipeId || Date.now(),
            recipeId
          )
        ) {
          return false;
        }

        return true;
      });
    },

    getRecipeIngredientUnit(recipeId) {
      if (!recipeId) return null;
      const recipe = this.ingredients.find((r) => r.id === Number(recipeId));
      return recipe?.yield?.unit || null;
    },
    updateRecipeIngredientUnit(index) {
      const ing = this.newIngredient.ingredients[index];
      if (ing.type === "recipe" && ing.recipeId) {
        const recipe = this.ingredients.find(
          (r) => r.id === Number(ing.recipeId)
        );
        if (recipe) {
          ing.unit = recipe.yield.unit;
        }
      }
    },
    deleteIngredient(ingredientId) {
      ingredientId = Number(ingredientId);
      if (
        confirm(
          "Are you sure you want to delete this ingredient? This will affect any recipes using it."
        )
      ) {
        this.ingredients = this.ingredients.filter(
          (i) => i.id !== ingredientId
        );
        // Update meals using this ingredient
        this.meals.forEach((meal) => {
          meal.ingredients = meal.ingredients.filter(
            (i) => i.type !== "recipe" || Number(i.recipeId) !== ingredientId
          );
        });
        localStorage.setItem("ingredients", JSON.stringify(this.ingredients));
        localStorage.setItem("meals", JSON.stringify(this.meals));
      }
    },
    // For meals
    addMealIngredient() {
      this.newMeal.ingredients.push({
        type: "basic",
        name: "",
        amount: "",
        unit: "",
      });
    },

    removeMealIngredient(index) {
      this.newMeal.ingredients.splice(index, 1);
    },

    // For recipe ingredients
    addRecipeIngredient() {
      this.newIngredient.ingredients.push({
        type: "basic",
        name: "",
        amount: "",
        unit: "",
      });
    },

    removeRecipeIngredient(index) {
      this.newIngredient.ingredients.splice(index, 1);
    },
    getMealById(mealId) {
      return this.meals.find((meal) => meal.id === Number(mealId));
    },

    getScheduledMealsForDate(date) {
      return this.schedules
        .filter((schedule) => {
          const scheduleDate = this.DateTime.fromISO(schedule.date);
          return scheduleDate.hasSame(date, "day");
        })
        .sort((a, b) => a.time.localeCompare(b.time));
    },
    calculateTotalQuantities(recipeId, amount = 1) {
      const recipe = this.ingredients.find((r) => r.id === Number(recipeId));
      if (!recipe) return {};

      const quantities = {};

      recipe.ingredients.forEach((ing) => {
        if (ing.type === "basic") {
          const key = `${ing.name}-${ing.unit}`;
          quantities[key] = (quantities[key] || 0) + ing.amount * amount;
        } else if (ing.type === "recipe" && ing.recipeId) {
          const subRecipe = this.ingredients.find(
            (r) => r.id === Number(ing.recipeId)
          );
          if (subRecipe) {
            const multiplier = (ing.amount * amount) / subRecipe.yield.amount;
            const subQuantities = this.calculateTotalQuantities(
              ing.recipeId,
              multiplier
            );

            Object.entries(subQuantities).forEach(([key, value]) => {
              quantities[key] = (quantities[key] || 0) + value;
            });
          }
        }
      });

      return quantities;
    },
    generateShoppingList() {
      if (!this.shoppingListStartDate || !this.shoppingListEndDate) {
        alert("Please select both start and end dates");
        return;
      }

      const start = this.DateTime.fromISO(this.shoppingListStartDate);
      const end = this.DateTime.fromISO(this.shoppingListEndDate);

      if (end < start) {
        alert("End date must be after start date");
        return;
      }

      const schedulesInRange = this.schedules.filter((schedule) => {
        const scheduleDate = this.DateTime.fromISO(schedule.date);
        return scheduleDate >= start && scheduleDate <= end;
      });

      // Helper function to process recipe ingredients recursively
      const processIngredients = (ingredients, multiplier = 1) => {
        const result = {};

        ingredients.forEach((ing) => {
          if (ing.type === "recipe") {
            const recipe = this.ingredients.find(
              (r) => r.id === Number(ing.recipeId)
            );
            if (recipe) {
              // Calculate the multiplier based on the recipe yield
              const recipeMultiplier =
                (ing.amount / recipe.yield.amount) * multiplier;
              const subIngredients = processIngredients(
                recipe.ingredients,
                recipeMultiplier
              );

              // Merge sub-ingredients into result
              Object.entries(subIngredients).forEach(([key, value]) => {
                if (!result[key]) {
                  result[key] = { ...value };
                } else {
                  result[key].amount += value.amount;
                }
              });
            }
          } else {
            const key = `${ing.name}-${ing.unit}`;
            if (!result[key]) {
              result[key] = {
                ingredient: ing.name,
                amount: 0,
                unit: ing.unit,
              };
            }
            result[key].amount += (parseFloat(ing.amount) || 0) * multiplier;
          }
        });

        return result;
      };

      // Process all scheduled meals
      const ingredients = {};
      schedulesInRange.forEach((schedule) => {
        const meal = this.getMealById(Number(schedule.mealId));
        if (!meal) return;

        const mealIngredients = processIngredients(meal.ingredients);
        Object.entries(mealIngredients).forEach(([key, value]) => {
          if (!ingredients[key]) {
            ingredients[key] = { ...value };
          } else {
            ingredients[key].amount += value.amount;
          }
        });
      });

      this.shoppingList = Object.values(ingredients);
    },

    // New methods for ingredient management
    openCreateIngredientModal() {
      this.editingIngredientId = null;
      this.newIngredient = {
        name: "",
        recipeUrl: "",
        ingredients: [],
        yield: { amount: 1, unit: "unit" },
      };
      this.showIngredientModal = true;
    },

    editIngredient(ingredient) {
      this.editingIngredientId = Number(ingredient.id);
      this.newIngredient = {
        name: ingredient.name,
        recipeUrl: ingredient.recipeUrl,
        ingredients: [...ingredient.ingredients],
        yield: { ...ingredient.yield },
      };
      this.showIngredientModal = true;
    },
  };
}
