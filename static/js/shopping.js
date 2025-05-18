document.addEventListener("alpine:init", () => {
  Alpine.data("shoppingList", () => ({
    init() {
      // Initialize any required state for shopping lists
    },

    deleteShoppingList(id, name) {
      if (confirm(`Are you sure you want to delete the shopping list "${name}"?`)) {
        htmx.ajax("DELETE", `/shopping-lists/${id}`, {
          target: "#shopping-lists-container",
          swap: "innerHTML",
          handler: (_, xhr) => {
            if (xhr.xhr.status >= 200 && xhr.xhr.status < 300) {
              // Show success message
              this.showAlert("Shopping list deleted successfully", "success");
              
              // If on the detail page, redirect to lists page
              if (window.location.pathname.includes(`/shopping-lists/${id}`)) {
                window.location.href = "/shopping-lists";
              }
            } else {
              this.showAlert("Error deleting shopping list", "error");
            }
          },
        });
      }
    },

    removeMealFromList(listId, mealId, mealName) {
      if (confirm(`Remove "${mealName}" from this shopping list?`)) {
        htmx.ajax("DELETE", `/shopping-lists/${listId}/meals/${mealId}`, {
          target: "#items-container",
          swap: "innerHTML",
          handler: (_, xhr) => {
            if (xhr.xhr.status >= 200 && xhr.xhr.status < 300) {
              this.showAlert("Meal removed successfully", "success");
              
              // Refresh the meals container as well
              htmx.ajax("GET", `/shopping-lists/${listId}`, {
                target: "#meals-container",
                selector: "#meals-container"
              });
            } else {
              this.showAlert("Error removing meal", "error");
            }
          },
        });
      }
    },

    deleteShoppingListItem(listId, itemId) {
      if (confirm("Remove this item from the shopping list?")) {
        htmx.ajax("DELETE", `/shopping-lists/${listId}/items/${itemId}`, {
          target: "#items-container",
          swap: "innerHTML",
          handler: (_, xhr) => {
            if (xhr.xhr.status >= 200 && xhr.xhr.status < 300) {
              this.showAlert("Item removed successfully", "success");
            } else {
              this.showAlert("Error removing item", "error");
            }
          },
        });
      }
    },

    toggleItemPurchased(itemId, purchased) {
      // Update item purchased status in UI
      // This could be expanded to update the backend if needed
      const label = document.querySelector(`label[for="item-${itemId}"]`);
      
      if (purchased) {
        label.classList.add("line-through", "text-gray-500");
      } else {
        label.classList.remove("line-through", "text-gray-500");
      }
    },

    showPurchaseForm(itemId) {
      // Show a small form to record actual purchase quantity and price
      const promptAmount = prompt("Enter actual quantity purchased:", "");
      if (promptAmount === null) return; // User cancelled
      
      const promptPrice = prompt("Enter price (optional):", "");
      if (promptPrice === null) return; // User cancelled
      
      const listId = window.location.pathname.split("/").pop();
      const url = `/shopping-lists/${listId}/items/${itemId}/purchase`;
      
      htmx.ajax("POST", url, {
        target: "#items-container",
        values: {
          actual_quantity: promptAmount,
          price: promptPrice || "0"
        },
        handler: (_, xhr) => {
          if (xhr.xhr.status >= 200 && xhr.xhr.status < 300) {
            this.showAlert("Purchase recorded", "success");
          } else {
            this.showAlert("Error recording purchase", "error");
          }
        },
      });
    },

    showAlert(message, type) {
      // Dispatch an event to show an alert
      // The alert component is defined in app.js/base.templ
      window.dispatchEvent(
        new CustomEvent("show-alert", {
          detail: { message, type },
        })
      );
    }
  }));
});