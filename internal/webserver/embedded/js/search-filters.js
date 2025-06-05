const searchFilters = document.getElementById('search-filters')
const searchFiltersForm = searchFilters.closest('form')

searchFiltersForm.addEventListener('submit', event => {
  event.preventDefault()

  event.target.querySelectorAll('.date-control').forEach(function (el) {
      if (el.getElementsByClassName('input-year')[0].value === '' || el.getElementsByClassName('input-year')[0].value === '0') return
      let composed = el.getElementsByClassName('date')[0]
      composed.value = el.getElementsByClassName('input-year')[0].value.padStart(4, '0') + '-' + el.getElementsByClassName('input-month')[0].value + '-' + el.getElementsByClassName('input-day')[0].value.padStart(2, '0')
  })

  Array.from(searchFilters.querySelectorAll('input[name]')).forEach(input => {
    if (input.value === '' || input.value === '0') {
      input.setAttribute('disabled', 'disabled')
    }
  })

  const pubDateFrom = document.getElementById('pub-date-from')
  const pubDateTo = document.getElementById('pub-date-to')

  if (pubDateFrom.value !== '' && pubDateTo.value !== '') {
    if (pubDateFrom.value > pubDateTo.value) {
      let dateControl = pubDateFrom.closest('.date-control')
      dateControl.querySelector('.error-from-date-later-than-to-date').classList.remove('d-none')
      dateControl.querySelectorAll('input, select').forEach(function (el) {
        el.classList.add('invalid')
      })
      return
    }
  }
  searchFiltersForm.submit()
})
