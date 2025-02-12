document.addEventListener('alpine:init', () => {
    Alpine.data('foodManager', () => ({
        foods: [],
        searchQuery: '',
        typeFilter: 'all',
        
        initFoods({ foods }) {
            this.foods = foods;
        },

        get filteredFoods() {
            return this.foods.filter(food => {
                const matchesSearch = food.name
                    .toLowerCase()
                    .includes(this.searchQuery.toLowerCase());
                const matchesType =
                    this.typeFilter === 'all' ||
                    (this.typeFilter === 'recipe' && food.isRecipe) ||
                    (this.typeFilter === 'basic' && !food.isRecipe);
                return matchesSearch && matchesType;
            });
        },

        handleSearch() {
            // Optional: Add server-side search for large datasets
            htmx.trigger('#food-list', 'refreshFoods');
        },

        confirmDelete(food) {
            const confirmed = window.confirm(`Are you sure you want to delete ${food.name}?`);
            if (confirmed) {
                htmx.trigger(`button[hx-delete="/foods/${food.id}"]`, 'confirmed');
            }
        },

        handleFoodDeleted({ foodId }) {
            this.foods = this.foods.filter(f => f.id !== foodId);
        },

        openViewModal(foodId) {
            htmx.trigger('body', 'showFoodModal', { foodId });
        },

        openNewFoodModal() {
            htmx.trigger('body', 'showFoodModal', { foodId: null });
        }
    }));
});