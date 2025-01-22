document.addEventListener('alpine:init', () => {
  Alpine.store('units', {
    // Unit definitions and conversion rates
    unitTypes: {
      mass: {
        base: 'grams',
        units: {
          grams: { rate: 1 },
          kilograms: { rate: 1000 },
          ounces: { rate: 28.3495 },
          pounds: { rate: 453.592 }
        }
      },
      volume: {
        base: 'milliliters',
        units: {
          milliliters: { rate: 1 },
          liters: { rate: 1000 },
          teaspoons: { rate: 4.92892 },
          tablespoons: { rate: 14.7868 },
          cups: { rate: 236.588 },
          fluidOunces: { rate: 29.5735 }
        }
      },
      count: {
        base: 'pieces',
        units: {
          pieces: { rate: 1 },
          servings: { rate: 1 }
        }
      }
    },

    // Get all available units for a type
    getUnitsForType(type) {
      return Object.keys(this.unitTypes[type]?.units || {});
    },

    // Get all units
    getAllUnits() {
      return Object.values(this.unitTypes).flatMap(type => 
        Object.keys(type.units)
      );
    },

    // Get the type of a unit
    getUnitType(unit) {
      for (const [type, data] of Object.entries(this.unitTypes)) {
        if (data.units[unit]) return type;
      }
      return null;
    },

    // Check if units are compatible
    areUnitsCompatible(unit1, unit2) {
      return this.getUnitType(unit1) === this.getUnitType(unit2);
    },

    // Convert between units
    convert(value, fromUnit, toUnit, density = null) {
      const fromType = this.getUnitType(fromUnit);
      const toType = this.getUnitType(toUnit);

      if (!fromType || !toType) {
        throw new Error('Invalid units');
      }

      // Direct conversion within same type
      if (fromType === toType) {
        const { units, base } = this.unitTypes[fromType];
        const baseValue = value * units[fromUnit].rate;
        return baseValue / units[toUnit].rate;
      }

      // Mass/volume conversion if density is provided
      if (density && ((fromType === 'mass' && toType === 'volume') || 
                     (fromType === 'volume' && toType === 'mass'))) {
        // Convert to base units first
        const fromBase = value * this.unitTypes[fromType].units[fromUnit].rate;
        // Convert between mass and volume using density
        const toBase = fromType === 'mass' ? fromBase / density : fromBase * density;
        // Convert to target unit
        return toBase / this.unitTypes[toType].units[toUnit].rate;
      }

      throw new Error('Incompatible units');
    },

    // Format quantity for display
    formatQuantity(value) {
      if (value >= 100) return Math.round(value);
      if (value >= 10) return Math.round(value * 10) / 10;
      if (value >= 1) return Math.round(value * 100) / 100;
      return Math.round(value * 1000) / 1000;
    },

    // Suggest best unit based on quantity
    suggestUnit(value, currentUnit) {
      const type = this.getUnitType(currentUnit);
      if (!type) return currentUnit;

      const baseValue = this.convert(value, currentUnit, this.unitTypes[type].base);

      switch (type) {
        case 'mass':
          if (baseValue >= 1000) return 'kilograms';
          if (baseValue < 1) return 'grams';
          if (baseValue >= 453.592) return 'pounds';
          if (baseValue >= 28.3495) return 'ounces';
          return 'grams';

        case 'volume':
          if (baseValue >= 1000) return 'liters';
          if (baseValue < 1) return 'milliliters';
          if (baseValue >= 236.588) return 'cups';
          if (baseValue >= 14.7868) return 'tablespoons';
          if (baseValue >= 4.92892) return 'teaspoons';
          return 'milliliters';

        default:
          return currentUnit;
      }
    }
  });
});