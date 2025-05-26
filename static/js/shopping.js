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

    toggleItemPurchased(itemId, purchased) {
      // Get current list ID from URL
      const pathParts = window.location.pathname.split('/');
      const listId = pathParts[pathParts.length - 1];
      
      // If marking as purchased, prompt for actual details
      let actualQuantity = "";
      let actualPrice = "";
      
      if (purchased) {
        actualQuantity = prompt("Enter actual quantity purchased (optional):", "");
        if (actualQuantity === null) return; // User cancelled
        
        actualPrice = prompt("Enter price paid (optional):", "");
        if (actualPrice === null) return; // User cancelled
      }

      htmx.ajax("POST", `/shopping-lists/${listId}/items/${itemId}/purchased`, {
        target: "#items-container",
        values: {
          purchased: purchased,
          actual_quantity: actualQuantity || "0",
          actual_price: actualPrice || "0"
        },
        handler: (_, xhr) => {
          if (xhr.xhr.status >= 200 && xhr.xhr.status < 300) {
            this.showAlert(
              purchased ? "Item marked as purchased" : "Item unmarked", 
              "success"
            );
          } else {
            this.showAlert("Error updating item", "error");
          }
        },
      });
    },

    showItemEditModal(itemId) {
      // Get current values from the item display
      const itemElement = document.querySelector(`[data-item-id="${itemId}"]`);
      if (!itemElement) return;
      
      const currentQuantity = prompt("Enter new quantity:", "");
      if (currentQuantity === null) return; // User cancelled
      
      const currentNotes = prompt("Enter notes (optional):", "");
      if (currentNotes === null) return; // User cancelled
      
      // Get current list ID from URL
      const pathParts = window.location.pathname.split('/');
      const listId = pathParts[pathParts.length - 1];

      htmx.ajax("PUT", `/shopping-lists/${listId}/items/${itemId}`, {
        target: "#items-container",
        values: {
          quantity: currentQuantity,
          notes: currentNotes || ""
        },
        handler: (_, xhr) => {
          if (xhr.xhr.status >= 200 && xhr.xhr.status < 300) {
            this.showAlert("Item updated successfully", "success");
          } else {
            this.showAlert("Error updating item", "error");
          }
        },
      });
    },

    deleteShoppingListItem(listId, itemId) {
      if (confirm("Remove this item from the shopping list?")) {
        htmx.ajax("DELETE", `/shopping-lists/${listId}/items/${itemId}`, {
          target: "#items-container",
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

    removeItemsBySource(listId, sourceId, sourceName) {
      if (confirm(`Remove all items from "${sourceName}"?`)) {
        htmx.ajax("DELETE", `/shopping-lists/${listId}/sources/${sourceId}`, {
          target: "#items-container",
          handler: (_, xhr) => {
            if (xhr.xhr.status >= 200 && xhr.xhr.status < 300) {
              this.showAlert("Items removed successfully", "success");
              
              // Also refresh the sources container
              htmx.ajax("GET", `/shopping-lists/${listId}`, {
                target: "#sources-container",
                selector: "#sources-container"
              });
            } else {
              this.showAlert("Error removing items", "error");
            }
          },
        });
      }
    },

    // Export functionality with better UX
    exportShoppingList(listId, listName) {
      // Create a temporary link to trigger download
      const link = document.createElement('a');
      link.href = `/shopping-lists/${listId}/export`;
      link.download = `${listName.replace(/[^a-z0-9]/gi, '_').toLowerCase()}_shopping_list.txt`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      
      this.showAlert("Shopping list exported", "success");
    },

    // Helper method for showing alerts
    showAlert(message, type = "info") {
      // Create alert element
      const alertId = `alert-${Date.now()}`;
      const alertClass = {
        success: "bg-green-100 border-green-400 text-green-700",
        error: "bg-red-100 border-red-400 text-red-700",
        info: "bg-blue-100 border-blue-400 text-blue-700",
        warning: "bg-yellow-100 border-yellow-400 text-yellow-700"
      }[type] || "bg-gray-100 border-gray-400 text-gray-700";

      const alertHTML = `
        <div id="${alertId}" class="fixed top-4 right-4 z-50 max-w-sm w-full">
          <div class="border-l-4 p-4 rounded shadow-lg ${alertClass}">
            <div class="flex">
              <div class="flex-1">
                <p class="text-sm font-medium">${message}</p>
              </div>
              <button onclick="document.getElementById('${alertId}').remove()" class="ml-3">
                <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                  <path d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"/>
                </svg>
              </button>
            </div>
          </div>
        </div>
      `;

      // Add to DOM
      document.body.insertAdjacentHTML('beforeend', alertHTML);

      // Auto-remove after 3 seconds
      setTimeout(() => {
        const alertElement = document.getElementById(alertId);
        if (alertElement) {
          alertElement.remove();
        }
      }, 3000);
    },

    // Quick add functionality for power users
    quickAddItem(listId) {
      const foodName = prompt("Enter food name:");
      if (!foodName) return;
      
      const quantity = prompt("Enter quantity:", "1");
      if (!quantity) return;
      
      const unit = prompt("Enter unit (e.g., pieces, cups, lbs):", "pieces");
      if (!unit) return;

      // This would require a new endpoint for quick add
      // For now, show alert that this feature needs implementation
      this.showAlert("Quick add feature coming soon! Use the 'Add Items' button for now.", "info");
    },

    // Batch operations
    markAllPurchased(listId) {
      if (confirm("Mark all items as purchased?")) {
        // This would require a new batch endpoint
        this.showAlert("Batch operations coming soon!", "info");
      }
    },

    clearPurchased(listId) {
      if (confirm("Remove all purchased items from the list?")) {
        // This would require a new batch endpoint
        this.showAlert("Batch operations coming soon!", "info");
      }
    }
  }));
});
