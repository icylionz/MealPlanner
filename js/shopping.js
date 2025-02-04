document.addEventListener('alpine:init', () => {
    Alpine.data('shoppingList', () => ({
        startDate: '',
        endDate: '',
        meals: [],

        init() {
            // Set default date range to current week
            const today = this.$store.mealPlanner.DateTime.local();
            this.startDate = today.toISODate();
            this.endDate = today.plus({ days: 7 }).toISODate();
        },

        toggleMeal(meal) {
            meal.expanded = !meal.expanded;
        },

        getFood(foodId) {
            return this.$store.foodManager.getFoodById(foodId);
        },

        formatDate(dateStr) {
            return this.$store.mealPlanner.DateTime.fromISO(dateStr).toFormat('ccc, MMM d • ');
        },
        
        generateList() {
               if (!this.startDate || !this.endDate) return;
       
               const start = this.$store.mealPlanner.DateTime.fromISO(this.startDate);
               const end = this.$store.mealPlanner.DateTime.fromISO(this.endDate);
       
               // Get all schedules within date range
               this.meals = this.$store.mealPlanner.schedules
                   .filter(schedule => {
                       const scheduleDate = this.$store.mealPlanner.DateTime.fromISO(schedule.date);
                       return scheduleDate >= start && scheduleDate <= end;
                   })
                   .map(schedule => ({
                       ...schedule,
                       food: this.processRecipeTree(this.$store.foodManager.getFoodById(schedule.foodId)),
                       expanded: false
                   }))
                   .sort((a, b) => {
                       const dateCompare = a.date.localeCompare(b.date);
                       return dateCompare || a.time.localeCompare(b.time);
                   });
           },
       
           processRecipeTree(food) {
               if (!food.isRecipe) return food;
       
               return {
                   ...food,
                   recipe: {
                       ...food.recipe,
                       ingredients: food.recipe.ingredients.map(ing => ({
                           ...ing,
                           expanded: false
                       }))
                   }
               };
           },
       
           toggleIngredient(ingredient) {
               ingredient.expanded = !ingredient.expanded;
           }
    }));
});