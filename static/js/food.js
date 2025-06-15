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
    confirmDeleteFood(food) {
      const confirmed = window.confirm(
        `Are you sure you want to delete ${food.name}?`,
      );
      if (confirmed) {
        htmx.ajax("DELETE", `/foods/${food.id}`, {
          handler: (_, xhr) => {
            if (xhr.xhr.status >= 200 && xhr.xhr.status < 300) {
              htmx.trigger("#food-list", "refreshFoodList");
            }
          },
        });
      }
    },

    // This section of code will remain in remembrance of the time wasted doing this. only to realize as soon 
    // as i finished, that i didn't need the yield unit select. It was so that everytime the user changed the 
    // unit type, the base unit select and yield unit select would change with the associated units for the type. 
    // e.g. user selects volume, base unit and yield unit would show milliliters and the other units of volume.
    // 
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
