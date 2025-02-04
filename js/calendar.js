document.addEventListener("alpine:init", () => {
  Alpine.data("calendar", () => ({
    // State
    isMobile: window.innerWidth < 640,
    selectedMobileDay: null,
    startHour: 0,
    endHour: 24,
    contextMenuPosition: { x: 0, y: 0 },
    showContextMenu: false,
    selectedDay: null,

    // Initialization
    init() {
      this.selectedMobileDay = this.$store.mealPlanner.currentDate;
      this.setupMobileDetection();
      this.setupContextMenuClose();
    },

    // Setup
    setupMobileDetection() {
      const updateMobileState = () => {
        const wasMobile = this.isMobile;
        this.isMobile = window.innerWidth < 640;

        // Hide context menu when switching between mobile and desktop
        if (wasMobile !== this.isMobile) {
          this.showContextMenu = false;
        }
      };
      window.addEventListener("resize", updateMobileState);
      updateMobileState();
    },

    setupContextMenuClose() {
      window.addEventListener("click", (e) => {
        if (
          !e.target.closest(".context-menu") &&
          !e.target.closest(".options-btn")
        ) {
          this.showContextMenu = false;
        }
      });
    },

    handleContextMenu(event, day) {
      event.preventDefault();
      this.showOptions(day, event);
    },

    // Computed properties
    get daysOfWeek() {
      return ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
    },

    get monthDays() {
      const store = this.$store.mealPlanner;
      const startOfMonth = store.DateTime.local(
        store.currentDate.year,
        store.currentDate.month,
        1,
      );
      const endOfMonth = startOfMonth.endOf("month");
      const start = startOfMonth.minus({ days: startOfMonth.weekday % 7 });

      const daysToAdd =
        (6 + 7 - (endOfMonth.weekday === 7 ? 0 : endOfMonth.weekday)) % 7;
      const end = endOfMonth.plus({ days: daysToAdd });

      const weeks = [];
      let current = start;

      while (current <= end) {
        const week = [];
        for (let i = 0; i < 7; i++) {
          week.push({
            date: current,
            isCurrentMonth: current.month === store.currentDate.month,
            schedules: this.getSchedulesForDay(current),
          });
          current = current.plus({ days: 1 });
        }
        weeks.push(week);
      }

      return weeks;
    },

    get weekDays() {
      const store = this.$store.mealPlanner;
      const startOfWeek = store.currentDate.startOf("week");
      return Array.from({ length: 7 }, (_, i) => {
        const date = startOfWeek.plus({ days: i });
        return {
          date,
          isCurrentMonth: date.hasSame(store.currentDate, 'month'),
          schedules: this.getSchedulesForDay(date)
        };
      });
    },

    get formattedCurrentRange() {
      const store = this.$store.mealPlanner;
      if (store.viewMode === "day") {
        return store.currentDate.toFormat("d MMM yyyy");
      } else if (store.viewMode === "week") {
        const startOfWeek = store.currentDate.startOf("week");
        const endOfWeek = store.currentDate.endOf("week");
        return `${startOfWeek.toFormat("d MMM")} - ${endOfWeek.toFormat("d MMM yyyy")}`;
      }
      return store.currentDate.toFormat("MMMM yyyy");
    },

    // Methods
    getSchedulesForDay(date) {
      const store = this.$store.mealPlanner;
      return store.schedules
        .filter((schedule) => {
          const scheduleDate = store.DateTime.fromISO(schedule.date);
          return scheduleDate.hasSame(date, "day");
        })
        .map((schedule) => ({
          ...schedule,
          food: this.$store.foodManager.getFoodById(schedule.foodId),
          style: this.getEventStyle(schedule.time),
        }))
        .sort((a, b) => a.time.localeCompare(b.time));
    },

    getEventStyle(time) {
      const [hours, minutes] = time.split(":").map(Number);
      const baseHeight = 6; // Consistent with our new rem-based system
      const topPosition = (hours + minutes / 60) * baseHeight;
      return `top: ${topPosition}rem; height: ${baseHeight}rem; position: absolute;`;
    },

    isCurrentDay(date) {
      return date.hasSame(this.$store.mealPlanner.DateTime.local(), "day");
    },

    formatDayName(date) {
      return date.toFormat("EEE");
    },

    formatDayNumber(date) {
      return date.toFormat("d");
    },

    // Navigation
    switchToDayView(date) {
      const store = this.$store.mealPlanner;
      store.currentDate = date;
      store.viewMode = "day";
    },

    switchToWeekView(date) {
      const store = this.$store.mealPlanner;
      // Handle both cases - when date is passed directly or as an object property
      const dateToUse = date.date || date;
      store.currentDate = dateToUse.startOf("week");
      store.viewMode = "week";
    },

    nextPeriod() {
      const store = this.$store.mealPlanner;
      const duration = {
        day: { days: 1 },
        week: { weeks: 1 },
        month: { months: 1 },
      }[store.viewMode];
      store.currentDate = store.currentDate.plus(duration);
    },

    previousPeriod() {
      const store = this.$store.mealPlanner;
      const duration = {
        day: { days: 1 },
        week: { weeks: 1 },
        month: { months: 1 },
      }[store.viewMode];
      store.currentDate = store.currentDate.minus(duration);
    },
    get timeGridConfig() {
      return {
        startHour: 0,
        endHour: 24,
        hourHeight: this.isMobile ? 64 : 96, // 16 or 24 rem in px
        get totalHeight() {
          return (this.endHour - this.startHour) * this.hourHeight;
        },
      };
    },

    // Time grid components
    timeGridHeader(date) {
      return `
            <div class="sticky top-0 z-10 bg-white border-b border-gray-200 p-4">
              <h3 class="text-lg font-semibold">${date.toFormat("cccc, d MMMM")}</h3>
            </div>
          `;
    },

    timeGridColumn() {
      const hours = Array.from(
        { length: this.timeGridConfig.endHour - this.timeGridConfig.startHour },
        (_, i) => i + this.timeGridConfig.startHour,
      );

      return `
            <div class="flex-none w-16 sm:w-24 bg-white border-r border-gray-200">
              <div class="h-14"></div>
              ${hours
                .map(
                  (hour) => `
                <div class="relative" style="height: ${this.timeGridConfig.hourHeight}px">
                  <div class="sticky top-0 text-xs sm:text-sm text-gray-500 px-2">
                    ${hour.toString().padStart(2, "0")}:00
                  </div>
                </div>
              `,
                )
                .join("")}
            </div>
          `;
    },
    // Generate time slots for 24 hours
    get timeSlots() {
      return Array.from({ length: 24 }, (_, i) => i);
    },

    // Format time with leading zeros
    formatTime(hour) {
      return `${hour.toString().padStart(2, "0")}:00`;
    },

    calculateEventStyle(schedule) {
      const [hours, minutes] = schedule.time.split(":").map(Number);
      return `top: ${(hours + minutes / 60) * 10}rem; height: 10rem;`; // Match grid height
    },

    // Get events for a specific day, sorted by time
    getEventsForDay(date) {
      // Get events for this day
      const events = this.$store.mealPlanner.schedules
        .filter(schedule => {
          const scheduleDate = this.$store.mealPlanner.DateTime.fromISO(schedule.date);
          return scheduleDate.hasSame(date, 'day');
        })
        .map(schedule => ({
          ...schedule,
          food: this.$store.foodManager.getFoodById(schedule.foodId),
        }))
        .sort((a, b) => a.time.localeCompare(b.time));
    
      // Group events by time slot
      const timeSlots = {};
      events.forEach(event => {
        if (!timeSlots[event.time]) {
          timeSlots[event.time] = [];
        }
        timeSlots[event.time].push(event);
      });
    
      // Calculate positions for overlapping events
      return events.map(event => {
        const eventsInSlot = timeSlots[event.time];
        const index = eventsInSlot.indexOf(event);
        const width = 100 / eventsInSlot.length;
        const left = index * width;
    
        return {
          ...event,
          style: `${this.calculateEventStyle(event)} width: ${width}%; left: ${left}%;`
        };
      });
    },

    get currentTimePosition() {
      const now = this.$store.mealPlanner.DateTime.local();
      const totalMinutes = now.hour * 60 + now.minute;
      // h-40 = 10rem, so divide hour slot by 60 minutes
      return `${totalMinutes * (10 / 60)}rem`;
    },

    get currentTimeFormatted() {
      const now = this.$store.mealPlanner.DateTime.local();
      return now.toFormat("HH:mm");
    },

    isWithinToday() {
      const now = this.$store.mealPlanner.DateTime.local();
      return this.$store.mealPlanner.currentDate.hasSame(now, "day");
    },

    openEventDetails(schedule) {
      window.dispatchEvent(new CustomEvent('show-meal-details', {
        detail: schedule
      }));
    },
    showDeleteScheduleOptions(schedules) {
        if (schedules.length === 1) {
            // Direct delete for single schedule
            this.confirmDeleteSchedule(schedules[0]);
        } else {
            // Show additional modal or expand context menu for multiple schedules
            const food = this.$store.foodManager.getFoodById(schedules[0].foodId);
            window.dispatchEvent(new CustomEvent('show-confirmation', {
                detail: {
                    title: 'Delete Schedules',
                    message: `Delete all ${schedules.length} scheduled meals for this day?`,
                    confirmText: 'Delete All',
                    onConfirm: () => this.deleteSchedules(schedules)
                }
            }));
        }
        this.showContextMenu = false;
    },
    
    confirmDeleteSchedule(schedule) {
        const food = this.$store.foodManager.getFoodById(schedule.foodId);
        window.dispatchEvent(new CustomEvent('show-confirmation', {
            detail: {
                title: 'Delete Schedule',
                message: `Are you sure you want to delete ${food?.name || 'this meal'} scheduled for ${schedule.time}?`,
                confirmText: 'Delete',
                onConfirm: () => this.deleteSchedule(schedule.id)
            }
        }));
    },
    
    deleteSchedule(scheduleId) {
        this.$store.mealPlanner.schedules = this.$store.mealPlanner.schedules.filter(
            schedule => schedule.id !== scheduleId
        );
    },
    
    deleteSchedules(schedules) {
        const scheduleIds = schedules.map(s => s.id);
        this.$store.mealPlanner.schedules = this.$store.mealPlanner.schedules.filter(
            schedule => !scheduleIds.includes(schedule.id)
        );
    },
    showEventOptions(event) {
        // Close other open menus
        this.getEventsForDay(this.$store.mealPlanner.currentDate)
            .forEach(e => {
                if (e !== event) e.showOptions = false;
            });
        event.showOptions = !event.showOptions;
    },
    
    showOptions(day, event) {
        // Close any open event menus first
        this.getEventsForDay(this.$store.mealPlanner.currentDate)
            .forEach(e => e.showOptions = false);
            
        if (event) {
            this.contextMenuPosition = {
                x: event.clientX,
                y: event.clientY,
            };
        }
        this.selectedDay = day;
        this.showContextMenu = true;
    },
    
    getDayData(date) {
        return {
            date: date,
            isCurrentDay: this.isCurrentDay(date),
            schedules: this.getSchedulesForDay(date)
        };
    },
  }));
  
  Alpine.data("scheduleManager", () => ({
    selectedFoodId: null,
    selectedTime: "12:00",
    errors: {},

    saveSchedule() {
      if (!this.selectedFoodId) {
        this.errors = { food: "Please select a food" };
        return;
      }

      const schedule = {
        id: String(Date.now()),
        foodId: this.selectedFoodId,
        date: this.$store.mealPlanner.selectedScheduleDate.toISODate(),
        time: this.selectedTime,
      };

      this.$store.mealPlanner.schedules.push(schedule);
      this.$store.mealPlanner.toggleScheduleModal(null);
    },
    formatScheduleDate(date) {
      return date ? date.toFormat('MMMM d, yyyy') : '';
    }
  }));
});
