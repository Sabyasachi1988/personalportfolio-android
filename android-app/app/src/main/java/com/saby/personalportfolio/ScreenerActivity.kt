package com.saby.personalportfolio

import android.content.Intent
import android.graphics.Typeface
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.text.Editable
import android.text.InputType
import android.text.SpannableStringBuilder
import android.text.Spanned
import android.text.TextWatcher
import android.text.style.ForegroundColorSpan
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.CheckBox
import android.widget.EditText
import android.widget.HorizontalScrollView
import android.widget.LinearLayout
import android.widget.PopupMenu
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.appcompat.view.ContextThemeWrapper
import androidx.core.content.ContextCompat
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.chip.Chip
import com.google.android.material.chip.ChipGroup
import com.ledger.bridge.Bridge

/**
 * Fund Screener: every open-ended Direct-Growth fund, downloaded once (about 2 MB) and then screened offline -
 * category, name search, return / risk / portfolio-mix filters, sorting - with multi-select for Compare and Track.
 * The data is built monthly by the fund-data repo (see build/screener.py there); nothing here needs a login or key.
 */
class ScreenerActivity : AppCompatActivity() {
    private val main = Handler(Looper.getMainLooper())
    private var all: List<ScreenerFund> = emptyList()
    private var shown: List<ScreenerFund> = emptyList()
    private var category: String? = "Mid Cap Fund"
    private var query = ""
    private var filters = ScreenerFilters()
    private var sort = ScreenerSort.R3
    private val selected = LinkedHashSet<Int>()

    private lateinit var subtitle: TextView
    private lateinit var chips: ChipGroup
    private lateinit var count: TextView
    private lateinit var list: RecyclerView
    private lateinit var filterBtn: Button
    private lateinit var sortBtn: Button
    private lateinit var bar: LinearLayout
    private lateinit var compareBtn: Button
    private lateinit var trackBtn: Button
    private lateinit var downloadBox: LinearLayout
    private lateinit var content: LinearLayout
    private lateinit var downloadBtn: Button
    private lateinit var downloadMsg: TextView
    private val adapter = FundAdapter()

    private fun dp(v: Int) = (v * resources.displayMetrics.density).toInt()
    private fun color(id: Int) = ContextCompat.getColor(this, id)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val root = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL; setPadding(dp(16), dp(16), dp(16), dp(8)) }
        root.addView(TextView(this).apply { text = "Fund Screener"; textSize = 20f; setTypeface(typeface, Typeface.BOLD) })
        subtitle = TextView(this).apply { textSize = 12f; setTextColor(color(R.color.colorNeutral)); setPadding(0, dp(2), 0, dp(8)) }
        root.addView(subtitle)

        downloadMsg = TextView(this).apply { textSize = 14f; gravity = Gravity.CENTER; setPadding(dp(8), dp(24), dp(8), dp(12)) }
        downloadBtn = Button(this).apply { text = "Download all funds"; setOnClickListener { download() } }
        downloadBox = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL; gravity = Gravity.CENTER_HORIZONTAL; visibility = View.GONE
            addView(downloadMsg); addView(downloadBtn)
        }
        root.addView(downloadBox, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))

        content = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL; visibility = View.GONE }
        val search = EditText(this).apply {
            hint = "Search name or fund house"; setSingleLine(); textSize = 14f
            inputType = InputType.TYPE_CLASS_TEXT
            addTextChangedListener(object : TextWatcher {
                override fun afterTextChanged(s: Editable?) { query = s?.toString().orEmpty(); refresh() }
                override fun beforeTextChanged(s: CharSequence?, a: Int, b: Int, c: Int) {}
                override fun onTextChanged(s: CharSequence?, a: Int, b: Int, c: Int) {}
            })
        }
        content.addView(search)
        chips = ChipGroup(this).apply { isSingleSelection = true; isSelectionRequired = true; setPadding(0, 0, dp(8), 0) }
        content.addView(HorizontalScrollView(this).apply { isHorizontalScrollBarEnabled = false; addView(chips) })

        val tools = LinearLayout(this).apply { gravity = Gravity.CENTER_VERTICAL }
        filterBtn = Button(this, null, com.google.android.material.R.attr.materialButtonOutlinedStyle).apply { setOnClickListener { showFilters() } }
        sortBtn = Button(this, null, com.google.android.material.R.attr.materialButtonOutlinedStyle).apply { setOnClickListener { showSort() } }
        tools.addView(filterBtn, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
        tools.addView(sortBtn, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f).apply { marginStart = dp(8) })
        content.addView(tools)
        count = TextView(this).apply { textSize = 12f; setTextColor(color(R.color.colorNeutral)); setPadding(dp(2), dp(6), 0, dp(4)) }
        content.addView(count)
        list = RecyclerView(this).apply { layoutManager = LinearLayoutManager(this@ScreenerActivity); adapter = this@ScreenerActivity.adapter; setHasFixedSize(false) }
        content.addView(list, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f))
        root.addView(content, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f))

        bar = LinearLayout(this).apply { gravity = Gravity.CENTER_VERTICAL; visibility = View.GONE; setPadding(0, dp(6), 0, dp(6)) }
        compareBtn = Button(this).apply { setOnClickListener { openCompare() } }
        trackBtn = Button(this, null, com.google.android.material.R.attr.materialButtonOutlinedStyle).apply { setOnClickListener { trackSelected() } }
        bar.addView(compareBtn, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
        bar.addView(trackBtn, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f).apply { marginStart = dp(8) })
        root.addView(bar)
        setContentView(root)
        loadData()
    }

    private fun loadData() {
        if (!ScreenerStore.isDownloaded(this)) {
            content.visibility = View.GONE; bar.visibility = View.GONE; downloadBox.visibility = View.VISIBLE
            subtitle.text = "Not downloaded yet"
            downloadMsg.text = "Download every Direct-Growth fund once (about 2 MB), then search, screen and compare them offline. It refreshes monthly."
            downloadBtn.isEnabled = true; downloadBtn.text = "Download all funds"
            return
        }
        Thread {
            val data = ScreenerStore.load(this)
            main.post {
                all = data
                downloadBox.visibility = View.GONE; content.visibility = View.VISIBLE
                subtitle.text = "${all.size} Direct-Growth funds · data as of ${ScreenerStore.asof}"
                buildChips()
                refresh()
            }
        }.start()
    }

    private fun download() {
        downloadBtn.isEnabled = false; downloadBtn.text = "Downloading…"
        Thread {
            val res = Bridge.downloadScreener(ScreenerStore.dir(this).absolutePath)
            main.post {
                if (res.trimStart().startsWith("{\"error\"")) {
                    downloadBtn.isEnabled = true; downloadBtn.text = "Try again"
                    Toast.makeText(this, "Download failed - check your connection and try again", Toast.LENGTH_LONG).show()
                } else loadData()
            }
        }.start()
    }

    private fun buildChips() {
        chips.removeAllViews()
        val ctx = ContextThemeWrapper(this, com.google.android.material.R.style.Widget_Material3_Chip_Filter)
        val counts = all.groupingBy { it.category }.eachCount()
        // equity-style categories first, by how many funds they hold
        val names = counts.entries.sortedByDescending { it.value }.map { it.key }
        fun addChip(label: String, value: String?) {
            chips.addView(Chip(ctx).apply {
                text = if (value == null) "All (${all.size})" else "$label (${counts[value]})"
                isCheckable = true
                isChecked = value == category
                setOnClickListener { category = value; refresh() }
            })
        }
        addChip("All", null)
        if (category != null && category !in counts) category = null
        names.forEach { addChip(it, it) }
        (chips.getChildAt(0) as? Chip)?.isChecked = category == null
    }

    private fun refresh() {
        shown = ScreenerLogic.apply(all, category, query, filters, sort)
        filterBtn.text = if (filters.activeCount > 0) "Filters (${filters.activeCount})" else "Filters"
        sortBtn.text = "Sort: ${sort.label}"
        count.text = "${shown.size} fund(s)" + if (selected.isNotEmpty()) " · ${selected.size} selected" else ""
        adapter.notifyDataSetChanged()
        updateBar()
    }

    private fun updateBar() {
        bar.visibility = if (selected.isEmpty()) View.GONE else View.VISIBLE
        compareBtn.text = "Compare (${selected.size})"
        compareBtn.isEnabled = selected.size >= 2
        trackBtn.text = "Track (${selected.size})"
        count.text = "${shown.size} fund(s)" + if (selected.isNotEmpty()) " · ${selected.size} selected" else ""
    }

    private fun showSort() {
        val menu = PopupMenu(this, sortBtn)
        ScreenerSort.values().forEachIndexed { i, s -> menu.menu.add(0, i, i, s.label) }
        menu.setOnMenuItemClickListener { sort = ScreenerSort.values()[it.itemId]; refresh(); true }
        menu.show()
    }

    private fun openCompare() {
        if (selected.size < 2) return
        startActivity(Intent(this, ScreenerCompareActivity::class.java).putExtra("codes", selected.toIntArray()))
    }

    private fun trackSelected() {
        val rows = all.filter { it.row.code in selected }.map { it.row }
        ScreenerTrack.trackAll(this, rows)
    }

    private fun num(e: EditText): Double? = e.text.toString().trim().replace(',', '.').toDoubleOrNull()

    private fun showFilters() {
        val box = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL; setPadding(dp(20), dp(8), dp(20), 0) }
        fun field(label: String, value: Double?): EditText {
            box.addView(TextView(this).apply { text = label; textSize = 12f; setTextColor(color(R.color.colorNeutral)); setPadding(0, dp(10), 0, 0) })
            return EditText(this).apply {
                inputType = InputType.TYPE_CLASS_NUMBER or InputType.TYPE_NUMBER_FLAG_DECIMAL or InputType.TYPE_NUMBER_FLAG_SIGNED
                setSingleLine(); textSize = 14f
                if (value != null) setText(value.toString().removeSuffix(".0"))
                box.addView(this)
            }
        }
        val f1 = field("1-year return at least (%)", filters.min1y)
        val f3 = field("3-year return at least (% a year)", filters.min3y)
        val f5 = field("5-year return at least (% a year)", filters.min5y)
        val fv = field("Volatility at most (% a year)", filters.maxVol)
        val fd = field("Worst fall (max drawdown) at most (%) - enter 30 for -30%", filters.maxDrawdown)
        val fs = field("Sharpe ratio at least", filters.minSharpe)
        val fa = field("Fund age at least (years)", filters.minAge)
        val fl = field("Large-cap share at least (%)  [funds with portfolio data]", filters.minLarge)
        val fm = field("Mid-cap share at least (%)", filters.minMid)
        val fsm = field("Small-cap share at most (%)", filters.maxSmall)
        val only = CheckBox(this).apply { text = "Only funds with portfolio (cap) data"; isChecked = filters.onlyWithCapMix }
        box.addView(only)
        AlertDialog.Builder(this).setTitle("Filters").setView(ScrollView(this).apply { addView(box) })
            .setPositiveButton("Apply") { _, _ ->
                filters = ScreenerFilters(num(f1), num(f3), num(f5), num(fv), num(fd)?.let { Math.abs(it) }, num(fs), num(fa), num(fl), num(fm), num(fsm), only.isChecked)
                refresh()
            }
            .setNeutralButton("Reset") { _, _ -> filters = ScreenerFilters(); refresh() }
            .setNegativeButton("Cancel", null).show()
    }

    private fun pct(v: Double?, plus: Boolean = true): String = if (v == null) "–" else (if (plus && v > 0) "+" else "") + "%.1f%%".format(v)

    private inner class FundAdapter : RecyclerView.Adapter<FundAdapter.VH>() {
        inner class VH(val box: LinearLayout, val check: CheckBox, val name: TextView, val meta: TextView, val ret: TextView, val risk: TextView) : RecyclerView.ViewHolder(box)

        override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): VH {
            val row = LinearLayout(parent.context).apply {
                orientation = LinearLayout.HORIZONTAL; gravity = Gravity.CENTER_VERTICAL
                setPadding(0, dp(8), 0, dp(8))
                layoutParams = RecyclerView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT)
            }
            val check = CheckBox(parent.context)
            val col = LinearLayout(parent.context).apply { orientation = LinearLayout.VERTICAL }
            val name = TextView(parent.context).apply { textSize = 14f; setTypeface(typeface, Typeface.BOLD); maxLines = 2; ellipsize = android.text.TextUtils.TruncateAt.END }
            val meta = TextView(parent.context).apply { textSize = 11f; setTextColor(color(R.color.colorNeutral)) }
            val ret = TextView(parent.context).apply { textSize = 12.5f; setPadding(0, dp(3), 0, 0) }
            val risk = TextView(parent.context).apply { textSize = 11.5f; setTextColor(color(R.color.colorNeutral)) }
            col.addView(name); col.addView(meta); col.addView(ret); col.addView(risk)
            row.addView(check); row.addView(col, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
            return VH(row, check, name, meta, ret, risk)
        }

        override fun getItemCount() = shown.size

        override fun onBindViewHolder(h: VH, position: Int) {
            val f = shown[position]
            val code = f.row.code
            h.name.text = f.shortName
            val capTxt = if (f.large != null) " · L ${"%.0f".format(f.large)} / M ${"%.0f".format(f.mid)} / S ${"%.0f".format(f.small)}" else ""
            h.meta.text = "${f.category} · ${"%.1f".format(f.row.age)} yrs$capTxt"
            val sb = SpannableStringBuilder()
            for ((label, key) in listOf("1Y" to "1y", "3Y" to "3y", "5Y" to "5y")) {
                val v = f.ret(key)
                sb.append("$label ")
                val start = sb.length
                sb.append(pct(v))
                if (v != null) sb.setSpan(ForegroundColorSpan(color(if (v >= 0) R.color.colorGain else R.color.colorLoss)), start, sb.length, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
                sb.append("    ")
            }
            h.ret.text = sb
            h.risk.text = "Vol ${pct(f.row.vol, false)}   Max fall ${pct(f.row.mdd, false)}   Sharpe ${f.row.sharpe?.let { "%.2f".format(it) } ?: "–"}"
            h.check.setOnCheckedChangeListener(null)
            h.check.isChecked = code in selected
            val toggle = {
                if (code in selected) selected.remove(code)
                else if (selected.size >= 6) Toast.makeText(this@ScreenerActivity, "Compare up to 6 funds at a time", Toast.LENGTH_SHORT).show()
                else selected.add(code)
                notifyItemChanged(h.adapterPosition)
                updateBar()
            }
            h.check.setOnClickListener { toggle() }
            h.box.setOnClickListener { toggle() }
        }
    }
}
