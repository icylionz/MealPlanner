document.addEventListener("alpine:init", () => {
  Alpine.data("foodManager", () => ({
    foods: [],
    searchQuery: "",
    typeFilter: "all",

    initFoods({ foods }) {
      this.foods = foods;
    },

    get filteredFoods() {
      return this.foods.filter((food) => {
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

    handleSearch() {
      // Optional: Add server-side search for large datasets
      htmx.trigger("#food-list", "refreshFoods");
    },

    confirmDelete(food) {
      const confirmed = window.confirm(
        `Are you sure you want to delete ${food.name}?`,
      );
      if (confirmed) {
        htmx.trigger(`button[hx-delete="/foods/${food.id}"]`, "confirmed");
      }
    },

    handleFoodDeleted({ foodId }) {
      this.foods = this.foods.filter((f) => f.id !== foodId);
    },

    toggleViewModal(foodId) {
      htmx.trigger("body", "showFoodModal", { foodId });
    },

    openNewFoodModal() {
      htmx.trigger("body", "showFoodModal", { foodId: null });
    },
    
    // This section of code will remain in remembrance of the time wasted doing this. only to realize as soon as i finished, that i didn't need the yield unit select
    // changeUnitType(unitType) {
    //   console.log(unitType);
    //   htmx.ajax("GET", `/foods/units?unit_type=${unitType}`, {
    //     handler: (_, xhr) => {
    //       if (xhr.xhr.status === 200) {
    //         response = xhr.xhr.response;
    //         baseUnitSelect = document.getElementById("base-unit-select");
    //         if (baseUnitSelect != null) baseUnitSelect.innerHTML = response;
    //         yieldUnitSelect = document.getElementById("yield-unit-select");
    //         if (yieldUnitSelect != null) yieldUnitSelect.innerHTML = response;
    //       }
    //     },
    //   });
    // },
  }));
});
