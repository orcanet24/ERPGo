<div class="sidebar">
    <!-- Sidebar user panel -->

    <div class="user-panel text-center">
        <div class="image">
            <?php $image = $this->session->userdata('image') ?>
            <img src="<?php echo base_url((!empty($image) ? $image : 'assets/img/icons/default.jpg')) ?>" class="img-circle"
                alt="User Image">
        </div>
        <div class="info">
            <p><?php echo $this->session->userdata('fullname') ?></p>
            <a href="#"><i class="fa fa-circle text-success"></i>
                <?php echo $this->session->userdata('user_level') ?></a>
        </div>
    </div>




    <!-- sidebar menu -->
    <ul class="sidebar-menu">

        <li class="treeview <?php echo (($this->uri->segment(1) == "home") ? "active" : null) ?>">
            <a href="<?php echo base_url('home') ?>"> <i class="ti-dashboard"></i>
                <span><?php echo display('dashboard') ?></span>
            </a>
        </li>

        <!-- Invoice menu start -->
        <?php if ($this->permission1->method('new_invoice', 'create')->access() || $this->permission1->method('manage_invoice', 'read')->access() || $this->permission1->method('pos_invoice', 'create')->access() || $this->permission1->method('gui_pos', 'create')->access() || $this->permission1->method('terms_list', 'read')->access()  || $this->permission1->method('terms_add', 'read')->access()) { ?>
            <li class="treeview <?php
                                if ($this->uri->segment('1') == ("add_invoice") || $this->uri->segment('1') == ("invoice_list") || $this->uri->segment('1') == ("pos_invoice") || $this->uri->segment('1') == ("gui_pos") || $this->uri->segment('1') == ("invoice_details") || $this->uri->segment('1') == ("invoice_pad_print") || $this->uri->segment('1') == ("pos_print") || $this->uri->segment('1') == ("invoice_edit") || $this->uri->segment('1') == ("terms_list")  || $this->uri->segment('1') == ("terms_add")) {
                                    echo "active";
                                } else {
                                    echo " ";
                                }
                                ?>">
                <a href="#">
                    <i class="fa fa-balance-scale"></i><span><?php echo display('invoice') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>
                <ul class="treeview-menu">
                    <?php if ($this->permission1->method('new_invoice', 'create')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("add_invoice")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href="<?php echo base_url('add_invoice') ?>"><?php echo display('new_invoice') ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('manage_invoice', 'read')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("invoice_list")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href="<?php echo base_url('invoice_list') ?>"><?php echo display('manage_invoice') ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('pos_invoice', 'create')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("gui_pos")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href="<?php echo base_url('gui_pos') ?>"><?php echo display('pos_invoice') ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('terms_list', 'create')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("terms_list")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href="<?php echo base_url('terms_list') ?>"><?php echo display('terms_list') ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('terms_add', 'create')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("terms_add")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href="<?php echo base_url('terms_add') ?>"><?php echo display('terms_add') ?></a></li>
                    <?php } ?>
                </ul>
            </li>
        <?php } ?>

        <?php if ($this->permission1->method('add_customer', 'create')->access() || $this->permission1->method('manage_customer', 'read')->access() || $this->permission1->method('credit_customer', 'read')->access() || $this->permission1->method('paid_customer', 'read')->access() || $this->permission1->method('customer_ledger', 'read')->access() || $this->permission1->method('customer_advance', 'create')->access()) { ?>
            <li
                class="treeview <?php echo (($this->uri->segment(1) == "add_customer" || $this->uri->segment(1) == "customer_list" || $this->uri->segment(1) == "credit_customer" || $this->uri->segment(1) == "paid_customer" || $this->uri->segment(1) == "edit_customer" || $this->uri->segment(1) == "customer_ledgerdata" || $this->uri->segment(1) == "customer_ledger" || $this->uri->segment(1) == "advance_receipt" || $this->uri->segment(1) == "customer_advance") ? "active" : '') ?>">

                <a href="javascript:void(0)">

                    <i class="metismenu-icon pe-7s-user"></i> <span><?php echo display('customer') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>

                <ul class="treeview-menu">

                    <?php if ($this->permission1->method('add_customer', 'create')->access()) { ?>
                        <li class="<?php echo (($this->uri->segment(1) == "add_customer") ? "active" : '') ?>">
                            <a href="<?php echo base_url('add_customer') ?>"
                                class="<?php echo (($this->uri->segment(1) == "add_customer") ? "active" : null) ?>">
                                <?php echo display('add_customer') ?>

                            </a>

                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('manage_customer', 'read')->access()) { ?>
                        <li class="<?php echo (($this->uri->segment(1) == "customer_list") ? "active" : '') ?>">
                            <a href="<?php echo base_url('customer_list') ?>">

                                <?php echo display('customer_list') ?>

                            </a>

                        </li>
                    <?php } ?>


                </ul>

            </li>
        <?php } ?>
        <!-- supplier menu part -->
        <?php if ($this->permission1->method('add_supplier', 'create')->access() || $this->permission1->method('manage_supplier', 'read')->access() || $this->permission1->method('supplier_ledger', 'read')->access() || $this->permission1->method('supplier_advance', 'create')->access()) { ?>
            <li
                class="treeview <?php echo (($this->uri->segment(1) == "add_supplier" || $this->uri->segment(1) == "supplier_list" || $this->uri->segment(1) == "edit_supplier" || $this->uri->segment(1) == "supplier_ledgerdata" || $this->uri->segment(1) == "supplier_ledger" || $this->uri->segment(1) == "supplier_advance") ? "active" : '') ?>">

                <a href="javascript:void(0)">

                    <i class="metismenu-icon fa fa-user-secret"></i> <span><?php echo display('supplier') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>

                <ul class="treeview-menu">

                    <?php if ($this->permission1->method('add_supplier', 'create')->access()) { ?>
                        <li class="<?php echo (($this->uri->segment(1) == "add_supplier") ? "active" : '') ?>">
                            <a href="<?php echo base_url('add_supplier') ?>"
                                class="<?php echo (($this->uri->segment(1) == "add_supplier") ? "active" : null) ?>">
                                <?php echo display('add_supplier') ?>

                            </a>

                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('manage_supplier', 'read')->access()) { ?>
                        <li class="<?php echo (($this->uri->segment(1) == "supplier_list") ? "active" : '') ?>">
                            <a href="<?php echo base_url('supplier_list') ?>">

                                <?php echo display('supplier_list') ?>

                            </a>

                        </li>
                    <?php } ?>



                </ul>

            </li>
        <?php } ?>

        <!-- product menu part -->
        <?php if ($this->permission1->method('create_product', 'create')->access() || $this->permission1->method('add_product_csv', 'create')->access() || $this->permission1->method('manage_product', 'read')->access() || $this->permission1->method('create_category', 'create')->access() || $this->permission1->method('manage_category', 'read')->access() || $this->permission1->method('add_unit', 'create')->access() || $this->permission1->method('manage_unit', 'read')->access()) { ?>
            <li
                class="treeview <?php echo (($this->uri->segment(1) == "category_form" || $this->uri->segment(1) == "category_list" || $this->uri->segment(1) == "unit_form" || $this->uri->segment(1) == "unit_list" || $this->uri->segment(1) == "product_form" || $this->uri->segment(1) == "product_list" || $this->uri->segment(1) == "barcode" || $this->uri->segment(1) == "qrcode" || $this->uri->segment(1) == "bulk_products" || $this->uri->segment(1) == "product_details") ? "active" : '') ?>">

                <a href="javascript:void(0)">

                    <i class="metismenu-icon fa fa-cubes"></i> <span><?php echo display('product') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>

                <ul class="treeview-menu">

                    <?php if ($this->permission1->method('category', 'create')->access()) { ?>
                        <li class="<?php echo (($this->uri->segment(1) == "category_form") ? "active" : '') ?>">
                            <a href="<?php echo base_url('category_form') ?>"> <?php echo display('add_category') ?>

                            </a>

                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('manage_category', 'read')->access() || $this->permission1->method('manage_category', 'update')->access() || $this->permission1->method('manage_category', 'delete')->access()) { ?>
                        <li class="<?php echo (($this->uri->segment(1) == "category_list") ? "active" : '') ?>">
                            <a href="<?php echo base_url('category_list') ?>">

                                <?php echo display('category_list') ?>

                            </a>

                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('unit', 'create')->access() || $this->permission1->method('unit', 'update')->access()) { ?>
                        <li class="<?php echo (($this->uri->segment(1) == "unit_form") ? "active" : '') ?>">
                            <a href="<?php echo base_url('unit_form') ?>"> <?php echo display('add_unit') ?>

                            </a>

                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('manage_unit', 'create')->access() || $this->permission1->method('manage_unit', 'read')->access() || $this->permission1->method('manage_unit', 'delete')->access() || $this->permission1->method('manage_unit', 'update')->access()) { ?>
                        <li class="<?php echo (($this->uri->segment(1) == "unit_list") ? "active" : '') ?>">
                            <a href="<?php echo base_url('unit_list') ?>">

                                <?php echo display('unit_list') ?>

                            </a>

                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('create_product', 'create')->access()) { ?>
                        <li class="<?php echo (($this->uri->segment(1) == "product_form") ? "active" : '') ?>">
                            <a href="<?php echo base_url('product_form') ?>">

                                <?php echo display('add_product') ?>

                            </a>

                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('add_product_csv', 'create')->access()) { ?>
                        <li class="<?php echo (($this->uri->segment(1) == "bulk_products") ? "active" : '') ?>">
                            <a href="<?php echo base_url('bulk_products') ?>">

                                <?php echo display('add_product_csv') ?>

                            </a>

                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('manage_product', 'read')->access()) { ?>
                        <li class="<?php echo (($this->uri->segment(1) == "product_list") ? "active" : '') ?>">
                            <a href="<?php echo base_url('product_list') ?>">

                                <?php echo display('manage_product') ?>

                            </a>

                        </li>
                    <?php } ?>

                </ul>

            </li>
        <?php } ?>



        <!-- Purchase menu start -->
        <?php if ($this->permission1->method('add_purchase', 'create')->access() || $this->permission1->method('manage_purchase', 'read')->access()) { ?>
            <li class="treeview <?php
                                if ($this->uri->segment('1') == ("add_purchase") || $this->uri->segment('1') == ("purchase_edit") || $this->uri->segment('1') == ("purchase_list") || $this->uri->segment('1') == ("purchase_details")) {
                                    echo "active";
                                } else {
                                    echo " ";
                                }
                                ?>">
                <a href="#">
                    <i class="ti-shopping-cart"></i><span><?php echo display('purchase') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>
                <ul class="treeview-menu">
                    <?php if ($this->permission1->method('add_purchase', 'create')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("add_purchase")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href="<?php echo base_url('add_purchase') ?>"><?php echo display('add_purchase') ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('manage_purchase', 'read')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("purchase_list")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href="<?php echo base_url('purchase_list') ?>"><?php echo display('manage_purchase') ?></a></li>
                    <?php } ?>
                </ul>
            </li>
        <?php } ?>
        <!-- Purchase menu end -->



        <!-- Stock menu start -->
        <?php if ($this->permission1->method('stock', 'read')->access() || $this->permission1->method('stock_report', 'read')->access()) { ?>
            <li class="treeview <?php
                                if ($this->uri->segment('1') == ("stock")) {
                                    echo "active";
                                } else {
                                    echo " ";
                                }
                                ?>">
                <a href="#">
                    <i class="ti-bar-chart"></i><span><?php echo display('stock') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>
                <ul class="treeview-menu">
                    <?php if ($this->permission1->method('stock_report', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("stock")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('stock') ?>"><?php echo display('stock_report') ?></a></]li>
                        <?php } ?>

                </ul>
            </li>
        <?php } ?>
        <!-- Stock menu end -->


        <!-- Account part star -->
        <?php if ($this->permission1->method('show_tree', 'read')->access() || $this->permission1->method('expenditure_statement', 'read')->access() || $this->permission1->method('bank_reconciliation_report', 'read')->access() || $this->permission1->method('receipt_payment', 'read')->access() || $this->permission1->method('fixedasset_schedule', 'read')->access() || $this->permission1->method('income_statement_form', 'read')->access() || $this->permission1->method('bank_reconciliation', 'read')->access() || $this->permission1->method('predefined_accounts', 'read')->access() ||  $this->permission1->method('subaccount_list', 'read')->access() || $this->permission1->method('financial_year', 'read')->access() || $this->permission1->method('opening_balance', 'create')->access() || $this->permission1->method('supplier_payment', 'create')->access() || $this->permission1->method('customer_receive', 'create')->access() || $this->permission1->method('service_payment', 'create')->access() || $this->permission1->method('debit_voucher', 'read')->access() || $this->permission1->method('credit_voucher', 'read')->access() || $this->permission1->method('aprove_v', 'read')->access() || $this->permission1->method('contra_voucher', 'read')->access() || $this->permission1->method('balance_sheet', 'read')->access() || $this->permission1->method('journal_voucher', 'read')->access() || $this->permission1->method('add_payment_method', 'create')->access() || $this->permission1->method('cash_adjustment', 'create')->access() || $this->permission1->method('payment_method_list', 'read')->access()) { ?>
            <li class="treeview <?php
                                if ($this->uri->segment('2') == ("voucher_report") ||  $this->uri->segment('1') == ("bank_reconciliation") ||  $this->uri->segment('1') == ("expenditure_statement") ||  $this->uri->segment('1') == ("expenditure_statement_report") || $this->uri->segment('1') == ("receipt_payment") || $this->uri->segment('1') == ("receipt_payment_report") || $this->uri->segment('1') == ("bank_reconciliation_report") ||  $this->uri->segment('1') == ("fixedasset_schedule") ||  $this->uri->segment('1') == ("fixed_assets_report") ||  $this->uri->segment('1') == ("income_statement_form") ||  $this->uri->segment('1') == ("income_statement") ||  $this->uri->segment('1') == ("aprove_v") || $this->uri->segment('1') == ("add_opening_balance") ||  $this->uri->segment('1') == ("edit_opening_balance") || $this->uri->segment('1') == ("edit_voucher") || $this->uri->segment('1') == ("create_debit_voucher") || $this->uri->segment('1') == ("predefined_accounts") || $this->uri->segment('1') == ("financial_year") || $this->uri->segment('1') == ("subaccount_list") || $this->uri->segment('1') == ("show_tree") || $this->uri->segment('1') == ("opening_balance") || $this->uri->segment('1') == ("cash_book") || $this->uri->segment('1') == ("cash_book_report") || $this->uri->segment('1') == ("day_book") || $this->uri->segment('1') == ("day_book_report") || $this->uri->segment('1') == ("bank_book") || $this->uri->segment('1') == ("bank_book_report") || $this->uri->segment('1') == ("general_ledger") || $this->uri->segment('1') == ("sub_ledger") || $this->uri->segment('1') == ("sub_ledger_report") || $this->uri->segment('1') == ("accounts_report_search") || $this->uri->segment('1') == ("trial_balance") || $this->uri->segment('1') == ("trial_balance_report") || $this->uri->segment('1') == ("profit_loss_report_search") || $this->uri->segment('1') == ("profit_loss_report") || $this->uri->segment('1') == ("cash_flow") || $this->uri->segment('1') == ("inventory_ledger") || $this->uri->seg... [truncated]
                                    echo "active";
                                } else {
                                    echo " ";
                                }
                                ?>">
                <a href="#">
                    <i class="fa fa-money"></i><span><?php echo display('accounts') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>
                <ul class="treeview-menu">
                    <?php if ($this->permission1->method('show_tree', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("show_tree")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('show_tree') ?>"><?php echo display('c_o_a'); ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('subaccount_list', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("subaccount_list")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('subaccount_list') ?>"><?php echo display('subaccount_list'); ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('predefined_accounts', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("predefined_accounts")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('predefined_accounts') ?>"><?php echo display('predefined_accounts'); ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('financial_year', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("financial_year")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('financial_year') ?>"><?php echo display('financial_year'); ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('opening_balance', 'create')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("opening_balance")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('opening_balance') ?>"><?php echo display('opening_balance'); ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('debit_voucher', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("debit_voucher")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('debit_voucher') ?>"><?php echo display('debit_voucher'); ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('credit_voucher', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("credit_voucher")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('credit_voucher') ?>"><?php echo display('credit_voucher'); ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('contra_voucher', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("contra_voucher")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('contra_voucher') ?>"><?php echo display('contra_voucher'); ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('journal_voucher', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("journal_voucher")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('journal_voucher') ?>"><?php echo display('journal_voucher'); ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('bank_reconciliation', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("bank_reconciliation")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('bank_reconciliation') ?>"><?php echo display('bank_reconciliation'); ?></a>
                        </li>
                    <?php } ?>


                    <?php if ($this->permission1->method('add_payment_method', 'create')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("add_payment_method")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('add_payment_method') ?>"><?php echo display('add_payment_method'); ?></a>
                        </li>
                    <?php } ?>

                    <?php if ($this->permission1->method('payment_method_list', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("payment_method_list")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('payment_method_list') ?>"><?php echo display('payment_method_list'); ?></a>
                        </li>
                    <?php } ?>

                    <?php if ($this->permission1->method('supplier_payment', 'create')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("supplier_payment")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('supplier_payment') ?>"><?php echo display('supplier_payment'); ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('customer_receive', 'create')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("customer_receive")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('customer_receive') ?>"><?php echo display('customer_receive'); ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('service_payment', 'create')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("service_payment")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('service_payment') ?>"><?php echo display('service_payment'); ?></a>
                        </li>
                    <?php } ?>

                    <?php if ($this->permission1->method('cash_adjustment', 'create')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("cash_adjustment")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('cash_adjustment') ?>"><?php echo display('cash_adjustment'); ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('aprove_v', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("aprove_v")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('aprove_v') ?>"><?php echo display('voucher_approval'); ?></a>