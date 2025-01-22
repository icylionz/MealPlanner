document.addEventListener("alpine:init", () => {
  Alpine.store("mealPlanner", {
    // Core state
    DateTime: luxon.DateTime,
    currentDate: luxon.DateTime.local(),
    activeTab: "calendar",
    viewMode: "week",
    schedules: [],
    meals: [],
    foods: [], // Just stores the food data

    init() {
      this.loadFromStorage();
      this.watchStorage();
    },

    // Basic storage operations
    loadFromStorage() {
      try {
        const savedSchedules = localStorage.getItem("schedules");
        const savedFoods = localStorage.getItem("foods");

        if (savedSchedules) {
          this.schedules = JSON.parse(savedSchedules).map((schedule) => ({
            ...schedule,
            id: String(schedule.id),
            mealId: String(schedule.mealId),
          }));
        }

        if (savedFoods) {
          this.foods = JSON.parse(savedFoods).map((food) => ({
            ...food,
            id: String(food.id), // Convert to string
            recipe: food.recipe
              ? {
                  ...food.recipe,
                  ingredients: food.recipe.ingredients.map((ing) => ({
                    ...ing,
                    foodId: String(ing.foodId), // Convert ingredient foodId to string
                  })),
                }
              : null,
          }));
        }
      } catch (error) {
        console.error("Error loading data:", error);
      }
    },

    watchStorage() {
      Alpine.effect(() => {
        localStorage.setItem("schedules", JSON.stringify(this.schedules));
        localStorage.setItem("foods", JSON.stringify(this.foods));
      });
    },
  });
});
