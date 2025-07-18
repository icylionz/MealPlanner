document.addEventListener("alpine:init", () => {
  Alpine.data("foodAutocomplete", () => ({
    searchQuery: "",
    selectedFoodId: "",
    selectedFoodName: "",
    results: [],
    showDropdown: false,
    loading: false,
    selectedIndex: -1,
    debounceTimer: null,

    init() {
      // Initialize with selected food if provided
      const hiddenInput = this.$el.querySelector('input[type="hidden"]');
      if (hiddenInput && hiddenInput.value) {
        this.selectedFoodId = hiddenInput.value;
        // If we have a selected food, show its name
        const textInput = this.$el.querySelector('input[type="text"]');
        if (textInput && textInput.dataset.selectedName) {
          this.searchQuery = textInput.dataset.selectedName;
          this.selectedFoodName = textInput.dataset.selectedName;
        }
      }
    },

    handleInput(value) {
      this.searchQuery = value;

      // Clear selection if user is typing
      if (value !== this.selectedFoodName) {
        this.selectedFoodId = "";
        this.selectedFoodName = "";
      }

      // Clear previous timer
      if (this.debounceTimer) {
        clearTimeout(this.debounceTimer);
      }

      // Set new timer
      this.debounceTimer = setTimeout(() => {
        this.search(value);
      }, 300);
    },

    handleFocus() {
      if (this.searchQuery === "") {
        this.loadRecentFoods();
      } else {
        this.showDropdown = true;
      }
    },

    async search(query) {
      if (query.length === 0) {
        this.loadRecentFoods();
        return;
      }

      this.loading = true;
      this.showDropdown = true;
      this.selectedIndex = -1;

      try {
        // Find the autocomplete container to get the hidden input
        let autocompleteContainer = this.$el;
        while (
          autocompleteContainer &&
          !autocompleteContainer.hasAttribute("x-data")
        ) {
          autocompleteContainer = autocompleteContainer.parentElement;
        }

        // Determine which endpoint to use based on field name
        let endpoint = "/foods/autocomplete";
        if (autocompleteContainer) {
          const hiddenInput = autocompleteContainer.querySelector(
            'input[type="hidden"]',
          );
          const fieldName = hiddenInput ? hiddenInput.name : "";

          if (fieldName === "recipe_id" || fieldName.includes("recipe")) {
            endpoint = "/foods/recipes-autocomplete";
          }
        }

        const response = await fetch(
          `${endpoint}?query=${encodeURIComponent(query)}&limit=10`,
        );
        if (response.ok) {
          const html = await response.text();
          this.parseResults(html);
        } else {
          console.error("Search failed:", response.status);
          this.results = [];
        }
      } catch (error) {
        console.error("Search error:", error);
        this.results = [];
      } finally {
        this.loading = false;
      }
    },

    async loadRecentFoods() {
      this.loading = true;
      this.showDropdown = true;
      this.selectedIndex = -1;
    
      try {
        // Find the autocomplete container to get the hidden input
        let autocompleteContainer = this.$el;
        while (autocompleteContainer && !autocompleteContainer.hasAttribute('x-data')) {
          autocompleteContainer = autocompleteContainer.parentElement;
        }
        
        // Determine which endpoint to use based on field name
        let endpoint = '/foods/recent';
        if (autocompleteContainer) {
          const hiddenInput = autocompleteContainer.querySelector('input[type="hidden"]');
          const fieldName = hiddenInput ? hiddenInput.name : '';
          
          if (fieldName === 'recipe_id' || fieldName.includes('recipe')) {
            endpoint = '/foods/recipes-autocomplete'; // Use recipe endpoint with empty query
          }
        }
    
        const response = await fetch(`${endpoint}?limit=10`);
        if (response.ok) {
          const html = await response.text();
          this.parseResults(html);
        }
      } catch (error) {
        console.error("Recent foods error:", error);
        this.results = [];
      } finally {
        this.loading = false;
      }
    },

    parseResults(html) {
      // Create temporary element to parse HTML
      const temp = document.createElement("div");
      temp.innerHTML = html;

      const foodElements = temp.querySelectorAll(".food-result");
      this.results = Array.from(foodElements).map((el) => ({
        id: el.dataset.foodId,
        name: el.dataset.foodName,
        isRecipe: el.dataset.foodIsRecipe === "true",
        baseUnit: el.dataset.foodBaseUnit,
      }));
    },

    selectFood(food) {
      this.selectedFoodId = food.id;
      this.selectedFoodName = food.name;
      this.searchQuery = food.name;
      this.closeDropdown();

      // Find the autocomplete container and update units dropdown
      let autocompleteContainer = this.$el;
      while (
        autocompleteContainer &&
        !autocompleteContainer.hasAttribute("x-data")
      ) {
        autocompleteContainer = autocompleteContainer.parentElement;
      }

      if (autocompleteContainer) {
        const hiddenInput = autocompleteContainer.querySelector(
          'input[type="hidden"]',
        );
        if (hiddenInput) {
          let unitsSelect = null;

          // Handle different form types
          if (hiddenInput.name.includes("ingredients[")) {
            // Recipe ingredient form
            const match = hiddenInput.name.match(/ingredients\[(\d+)\]/);
            if (match) {
              const index = match[1];
              unitsSelect = document.getElementById(`units-select-${index}`);
            }
          } else if (hiddenInput.name === "food_id") {
            // Manual shopping list item form
            unitsSelect = document.getElementById("manual-units");
          }

          if (unitsSelect) {
            htmx.ajax("GET", `/foods/units?food_id=${food.id}`, {
              target: unitsSelect,
              swap: "innerHTML",
            });
          }
        }
      }
    },

    navigateDown() {
      if (this.selectedIndex < this.results.length - 1) {
        this.selectedIndex++;
      }
    },

    navigateUp() {
      if (this.selectedIndex > 0) {
        this.selectedIndex--;
      }
    },

    selectCurrentItem() {
      if (this.selectedIndex >= 0 && this.selectedIndex < this.results.length) {
        this.selectFood(this.results[this.selectedIndex]);
      }
    },

    closeDropdown() {
      this.showDropdown = false;
      this.selectedIndex = -1;
    },
  }));
});
