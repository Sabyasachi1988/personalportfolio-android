package com.saby.personalportfolio

import android.view.View
import android.view.ViewGroup
import android.widget.FrameLayout
import androidx.recyclerview.widget.RecyclerView

/**
 * Hosts 3 already-inflated, persistent page views (built once in
 * AllocationActivity.onCreate) inside a ViewPager2. Deliberately NOT a
 * FragmentStateAdapter - this app has no Fragment architecture
 * elsewhere, and the pages' internal views (donut charts, RecyclerViews,
 * buttons) are simple enough that AllocationActivity can keep binding
 * data to them directly by field reference, exactly as before ViewPager2
 * was introduced, rather than needing a reactive per-page rebind
 * mechanism.
 *
 * Each page View can only have one parent at a time, so onBindViewHolder
 * detaches it from wherever it last lived before attaching it to this
 * ViewHolder's container - safe here since there are only 3 pages total
 * and RecyclerView's default ViewHolder pool comfortably holds all 3
 * without needing to actually recycle/rebind mid-session.
 */
class AllocationPagerAdapter(private val pages: List<View>) :
    RecyclerView.Adapter<AllocationPagerAdapter.PageHolder>() {

    class PageHolder(val container: FrameLayout) : RecyclerView.ViewHolder(container)

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): PageHolder {
        val container = FrameLayout(parent.context).apply {
            layoutParams = ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
        }
        return PageHolder(container)
    }

    override fun onBindViewHolder(holder: PageHolder, position: Int) {
        val page = pages[position]
        (page.parent as? ViewGroup)?.removeView(page)
        holder.container.removeAllViews()
        // Explicit LayoutParams here rather than relying on `page`'s own
        // (possibly null, since it was inflated with parent=null in
        // AllocationActivity) - addView(view, params) always applies
        // exactly the params passed, regardless of what the view already
        // had, guaranteeing each page actually fills its ViewPager2 page.
        holder.container.addView(page, ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
    }

    override fun getItemCount(): Int = pages.size
}
