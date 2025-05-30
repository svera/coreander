document.body.addEventListener('submit', function (evt) {
    evt.target.querySelectorAll('.date-control').forEach(function (el) {
        let composed = el.getElementsByClassName('date')[0]
        composed.value = el.getElementsByClassName('input-year')[0].value.padStart(4, '0') + '-' + document.getElementsByClassName('input-month')[0].value + '-' + document.getElementsByClassName('input-day')[0].value.padStart(2, '0')
    })
})

const searchFilters = document.getElementById('search-filters')

searchFilters.addEventListener('show.bs.collapse', event => {
  [...searchFilters.querySelectorAll('input[name]')].forEach(input => {
    input.removeAttribute('disabled')
  })
})

searchFilters.addEventListener('hide.bs.collapse', event => {
  [...searchFilters.querySelectorAll('input[name]')].forEach(input => {
    input.setAttribute('disabled', 'disabled')
  })
})
