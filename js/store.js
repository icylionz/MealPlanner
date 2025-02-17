document.addEventListener("alpine:init", () => {
  Alpine.data('confirmationModal', () => ({
      isVisible: false,
      title: '',
      message: '',
      confirmText: 'Confirm',
      cancelText: 'Cancel',
      onConfirm: () => {},
      onCancel: () => {},
  
      show({ title, message, confirmText, cancelText, onConfirm, onCancel }) {
          this.title = title;
          this.message = message;
          this.confirmText = confirmText || 'Confirm';
          this.cancelText = cancelText || 'Cancel';
          this.onConfirm = onConfirm;
          this.onCancel = onCancel;
          this.isVisible = true;
      },
  
      confirm() {
          this.onConfirm?.();
          this.isVisible = false;
      },
  
      cancel() {
          this.onCancel?.();
          this.isVisible = false;
      }
  }));
  Alpine.store("mealPlanner", {
    // Core state
    DateTime: luxon.DateTime,
    currentDate: luxon.DateTime.local(),
    activeTab: "calendar",
    viewMode: "month",
    schedules: [],
    meals: [],
    foods: [], // Just stores the food data
    showScheduleModal: false,
    selectedScheduleDate: null,
    showViewModal: false,
    viewingFoodId: null,
    showFoodModal: false,

    init() {
      this.showViewModal = false;
      this.viewingFoodId = null;
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

    toggleScheduleModal(date) {
      this.selectedScheduleDate = date;
      this.showScheduleModal = !this.showScheduleModal;
    },

    toggleViewModal(foodId) {
      this.viewingFoodId = foodId;
      this.showViewModal = !this.showViewModal;
    },

    closeViewModal() {
      this.showViewModal = false;
      this.viewingFoodId = null;
    },
  });
});
