"use strict"

const searchFilters = document.getElementById('search-filters')
const searchFiltersForm = searchFilters.closest('form')

console.log(searchFiltersForm, "SEARCH FILTERS FORM");
console.log(searchFilters, "SEARCH FILTERS");
searchFilters.querySelectorAll('.date-control').forEach(dateControl => {
    console.log(dateControl.querySelector('.input-month'), "DATE CONTROL");
});
/**
 * Determines if a given year is a leap year
 * @param {number} year - The year to check
 * @returns {boolean} - True if the year is a leap year, false otherwise
 */
function isLeapYear(year) {
    // A year is a leap year if:
    // 1. It's divisible by 4 AND
    // 2. It's either NOT divisible by 100 OR it's divisible by 400
    return (year % 4 === 0) && (year % 100 !== 0 || year % 400 === 0)
}

/**
 * Updates the max attribute of the day input based on the selected month and year
 * @param {HTMLElement} monthSelect - The month select element
 * @param {HTMLElement} dayInput - The day input element
 * @param {HTMLElement} yearInput - The year input element
 */
function updateMaxDays(monthSelect, dayInput, yearInput) {
    const month = parseInt(monthSelect.value)
    const year = parseInt(yearInput.value) || new Date().getFullYear()

    let maxDays = 31 // default max days

    switch (month) {
        case 2: // February
            maxDays = isLeapYear(year) ? 29 : 28
            break
        case 4: // April
        case 6: // June
        case 9: // September
        case 11: // November
            maxDays = 30
            break
    }

    // Update the max attribute
    dayInput.setAttribute('max', maxDays)

    // If current day value is greater than max days, set it to max days
    const currentDay = parseInt(dayInput.value)
    if (currentDay > maxDays) {
        dayInput.value = maxDays
    }
}

// Set up event listeners for all month selects
searchFilters.querySelectorAll('.date-control').forEach(dateControl => {
    const monthSelect = dateControl.querySelector('.input-month')
    const dayInput = dateControl.querySelector('.input-day')
    const yearInput = dateControl.querySelector('.input-year')

    // Update max days when month changes
    monthSelect.addEventListener('change', () => {
      console.log("Month changed to:", monthSelect.value)
        updateMaxDays(monthSelect, dayInput, yearInput)
    })

    // Update max days when year changes (for February)
    yearInput.addEventListener('change', () => {
        if (parseInt(monthSelect.value) === 2) { // Only update if February is selected
            updateMaxDays(monthSelect, dayInput, yearInput)
        }
    })

    // Initial update of max days
    updateMaxDays(monthSelect, dayInput, yearInput)
})

searchFiltersForm.addEventListener('submit', event => {
  event.preventDefault()

  searchFiltersForm.querySelectorAll('.date-control').forEach(function (el) {
      if (el.getElementsByClassName('input-year')[0].value === '' || el.getElementsByClassName('input-year')[0].value === '0') return
      let composed = el.getElementsByClassName('date')[0]
      let year = el.getElementsByClassName('input-year')[0].value
      if (year.startsWith('-') || year.startsWith('+')) {
        year = year.substring(0, 1) + year.substring(1).padStart(4, '0')
      } else {
        year = year.padStart(4, '0')
      }
      composed.value = year + '-' + el.getElementsByClassName('input-month')[0].value + '-' + el.getElementsByClassName('input-day')[0].value.padStart(2, '0')
  })

  Array.from(searchFilters.querySelectorAll('input')).forEach(input => {
    console.log(input.value, input.name)
    if (input.value === '' || input.value === '0') {
      input.setAttribute('disabled', 'disabled')
    }
  })

  //searchFiltersForm.submit()
})
