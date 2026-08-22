package com.saby.personalportfolio

import android.app.Activity
import android.content.Intent
import com.google.android.material.bottomnavigation.BottomNavigationView

enum class BottomNavDestination { DASHBOARD, HOLDINGS, ALLOCATION, TRANSACTIONS }

object BottomNavHelper {

    /**
     * Wires up [bottomNav] for [activity], marking [current] as selected
     * and navigating to the other screens on tap. Uses
     * FLAG_ACTIVITY_CLEAR_TOP + singleTop (declared in the manifest) so
     * repeatedly switching tabs reuses existing instances instead of
     * stacking up duplicate activities.
     */
    fun setup(activity: Activity, bottomNav: BottomNavigationView, current: BottomNavDestination) {
        val selectedId = when (current) {
            BottomNavDestination.DASHBOARD -> R.id.nav_dashboard
            BottomNavDestination.HOLDINGS -> R.id.nav_holdings
            BottomNavDestination.ALLOCATION -> R.id.nav_allocation
            BottomNavDestination.TRANSACTIONS -> R.id.nav_transactions
        }
        bottomNav.selectedItemId = selectedId

        bottomNav.setOnItemSelectedListener { item ->
            val target: Class<out Activity>? = when (item.itemId) {
                R.id.nav_dashboard -> if (current != BottomNavDestination.DASHBOARD) MainActivity::class.java else null
                R.id.nav_holdings -> if (current != BottomNavDestination.HOLDINGS) HoldingsActivity::class.java else null
                R.id.nav_allocation -> if (current != BottomNavDestination.ALLOCATION) AllocationActivity::class.java else null
                R.id.nav_transactions -> if (current != BottomNavDestination.TRANSACTIONS) TransactionsActivity::class.java else null
                else -> null
            }
            if (target != null) {
                val intent = Intent(activity, target)
                intent.addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP)
                activity.startActivity(intent)
            }
            true
        }
    }
}
