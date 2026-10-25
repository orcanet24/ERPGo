                        </li>
                    <?php } ?>

                    <?php if ($this->permission1->method('ac_report', 'create')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('2') == ("voucher_report") || $this->uri->segment('1') == ("expenditure_statement") ||  $this->uri->segment('1') == ("expenditure_statement_report") || $this->uri->segment('1') == ("bank_reconciliation_report") || $this->uri->segment('1') == ("receipt_payment") || $this->uri->segment('1') == ("receipt_payment_report") || $this->uri->segment('1') == ("income_statement_form") ||  $this->uri->segment('1') == ("income_statement") || $this->uri->segment('1') == ("fixedasset_schedule") ||  $this->uri->segment('1') == ("fixed_assets_report") || $this->uri->segment('1') == ("cash_book") || $this->uri->segment('1') == ("cash_book_report") || $this->uri->segment('1') == ("bank_book") || $this->uri->segment('1') == ("bank_book_report") || $this->uri->segment('1') == ("day_book") || $this->uri->segment('1') == ("day_book_report") || $this->uri->segment('1') == ("general_ledger")  || $this->uri->segment('1') == ("sub_ledger") || $this->uri->segment('1') == ("sub_ledger_report") || $this->uri->segment('1') == ("accounts_report_search") || $this->uri->segment('1') == ("trial_balance") || $this->uri->segment('1') == ("trial_balance_report") || $this->uri->segment('1') == ("profit_loss_form") || $this->uri->segment('1') == ("profit_loss_report_search") || $this->uri->segment('1') == ("profit_loss_report") || $this->uri->segment('1') == ("cash_flow") || $this->uri->segment('1') == ("inventory_ledger") || $this->uri->segment('1') == ("coa_print") || $this->uri->segment('1') == ("cashflow_form") || $this->uri->segment('1') == ("balance_sheet")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href=""><?php echo display('report') ?>
                                <span class="pull-right-container">
                                    <i class="fa fa-angle-left pull-right"></i>
                                </span>
                            </a>
                            <ul class="treeview-menu">
                                <?php if ($this->permission1->method('cash_book', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("cash_book")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('cash_book') ?>"><?php echo display('cash_book'); ?></a></li>
                                <?php } ?>


                                <?php if ($this->permission1->method('bank_book', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("bank_book")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('bank_book') ?>"><?php echo display('bank_book'); ?></a></li>
                                <?php } ?>
                                <?php if ($this->permission1->method('day_book', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("day_book")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('day_book') ?>"><?php echo display('day_book'); ?></a></li>
                                <?php } ?>
                                <?php if ($this->permission1->method('general_ledger', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("general_ledger") || $this->uri->segment('1') == ("general_ledger")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('general_ledger') ?>"><?php echo display('general_ledger'); ?></a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('sub_ledger', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("sub_ledger") || $this->uri->segment('1') == ("sub_ledger")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('sub_ledger') ?>"><?php echo display('sub_ledger'); ?></a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('trial_balance', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("trial_balance") || $this->uri->segment('1') == ("trial_balance")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('trial_balance') ?>"><?php echo display('trial_balance'); ?></a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('income_statement_form', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("income_statement_form") || $this->uri->segment('1') == ("income_statement_form")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('income_statement_form') ?>"><?php echo display('income_statement_form'); ?></a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('expenditure_statement', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("expenditure_statement") || $this->uri->segment('1') == ("expenditure_statement")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('expenditure_statement') ?>"><?php echo display('expenditure_statement'); ?></a>
                                    </li>
                                <?php } ?>
                                <li class="treeview <?php if ($this->uri->segment('1') == ("profit_loss_report") || $this->uri->segment('1') == ("profit_loss_report")) {
                                                        echo "active";
                                                    } else {
                                                        echo " ";
                                                    } ?>"><a href="<?php echo base_url('profit_loss_report') ?>"><?php echo display('profit_loss_report'); ?></a>
                                </li>
                                <?php if ($this->permission1->method('balance_sheet', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("balance_sheet")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('balance_sheet') ?>"><?php echo display('balance_sheet'); ?></a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('fixedasset_schedule', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("fixedasset_schedule")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('fixedasset_schedule') ?>"><?php echo display('fixedasset_schedule'); ?></a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('receipt_payment', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("receipt_payment")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('receipt_payment') ?>"><?php echo display('receipt_payment'); ?></a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('bank_reconciliation_report', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("bank_reconciliation_report")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('bank_reconciliation_report') ?>"><?php echo display('bank_reconciliation_report'); ?></a>
                                    </li>
                                <?php } ?>

                                <?php if ($this->permission1->method('coa_print', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("coa_print")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('coa_print') ?>"><?php echo display('coa_print'); ?></a></li>
                                <?php } ?>


                            </ul>

                        </li>
                    <?php } ?>
                </ul>
            </li>
        <?php } ?>
        <!--  Account End -->

        <!-- AI Business menu start -->
        <?php if ($this->permission1->method('ai_settings', 'create')->access() || $this->permission1->method('ai_business_analyst_report', 'read')->access()) { ?>
            <li class="treeview <?php
                                if ($this->uri->segment('1') == ("ai_settings") || $this->uri->segment('1') == ("ai_settings") || $this->uri->segment('1') == ("ai_settings") || $this->uri->segment('1') == ("ai_settings")) {
                                    echo "active";
                                } else {
                                    echo " ";
                                }
                                ?>">
                <a href="#">
                    <i class="fa fa-book"></i><span><?php echo display('ai_business_analyst') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>
                <ul class="treeview-menu">
                    <?php if ($this->permission1->method('ai_settings', 'create')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("ai_settings")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href="<?php echo base_url('ai_settings') ?>"><?php echo display('ai_setting') ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('ai_business_analyst_report', 'read')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("ai_business_analyst_report")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href="<?php echo base_url('open_ai') ?>"><?php echo display('ai_business_analyst_report') ?></a></li>
                    <?php } ?>
                </ul>
            </li>
        <?php } ?>
        <!-- AI Business menu end -->

        <!-- Report menu start -->
        <?php if ($this->permission1->method('add_closing', 'create')->access() || $this->permission1->method('closing_report', 'read')->access() || $this->permission1->method('todays_report', 'read')->access() || $this->permission1->method('todays_customer_receipt', 'read')->access() || $this->permission1->method('todays_sales_report', 'read')->access() || $this->permission1->method('due_report', 'read')->access() || $this->permission1->method('todays_purchase_report', 'read')->access() || $this->permission1->method('purchase_report_category_wise', 'read')->access() || $this->permission1->method('product_sales_reports_date_wise', 'read')->access() || $this->permission1->method('sales_report_category_wise', 'read')->access() || $this->permission1->method('shipping_cost_report', 'read')->access()) { ?>
            <li class="treeview <?php
                                if ($this->uri->segment('1') == ("closing_form") || $this->uri->segment('1') == ("closing_report") || $this->uri->segment('1') == ("closing_report_search") || $this->uri->segment('1') == ("todays_report") || $this->uri->segment('1') == ("todays_customer_received") || $this->uri->segment('1') == ("todays_customerwise_received") || $this->uri->segment('1') == ("sales_report") || $this->uri->segment('1') == ("datewise_sales_report") || $this->uri->segment('1') == ("userwise_sales_report") || $this->uri->segment('1') == ("invoice_wise_due_report") || $this->uri->segment('1') == ("shipping_cost_report") || $this->uri->segment('1') == ("purchase_report") || $this->uri->segment('1') == ("purchase_report_categorywise") || $this->uri->segment('1') == ("product_wise_sales_report") || $this->uri->segment('1') == ("category_sales_report") || $this->uri->segment('1') == ("sales_return") || $this->uri->segment('1') == ("supplier_returns") || $this->uri->segment('1') == ("tax_report") || $this->uri->segment('1') == ("profit_report")) {
                                    echo "active";
                                } else {
                                    echo " ";
                                }
                                ?>">
                <a href="#">
                    <i class="ti-book"></i><span><?php echo display('report') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>
                <ul class="treeview-menu">
                    <?php if ($this->permission1->method('add_closing', 'create')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("closing_form")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('closing_form') ?>"><?php echo display('closing') ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('closing_report', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("closing_report")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('closing_report') ?>"><?php echo display('closing_report') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('todays_report', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("todays_report")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('todays_report') ?>"><?php echo display('todays_report') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('todays_customer_receipt', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("todays_customer_received")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('todays_customer_received') ?>"><?php echo display('todays_customer_receipt') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('todays_sales_report', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("sales_report")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('sales_report') ?>"><?php echo display('sales_report') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('user_wise_sales_report', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("userwise_sales_report")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('userwise_sales_report') ?>"><?php echo display('user_wise_sales_report') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('due_report', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("invoice_wise_due_report")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('invoice_wise_due_report') ?>"><?php echo display('due_report'); ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('shipping_cost_report', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("shipping_cost_report")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('shipping_cost_report') ?>"><?php echo display('shipping_cost_report'); ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('purchase_report', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("purchase_report")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('purchase_report') ?>"><?php echo display('purchase_report') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('purchase_report_category_wise', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("purchase_report_categorywise")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('purchase_report_categorywise') ?>"><?php echo display('purchase_report_category_wise') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('sales_report_product_wise', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("product_wise_sales_report")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('product_wise_sales_report') ?>"><?php echo display('sales_report_product_wise') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('sales_report_category_wise', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("category_sales_report")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('category_sales_report') ?>"><?php echo display('sales_report_category_wise') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('invoice_return', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("sales_return")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('sales_return') ?>"><?php echo display('invoice_return') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('supplier_return', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("supplier_returns")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('supplier_returns') ?>"><?php echo display('supplier_return') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('tax_report', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("tax_report")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('tax_report') ?>"><?php echo display('tax_report') ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('profit_report', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("profit_report")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('profit_report') ?>"><?php echo display('profit_report') ?></a>
                        </li>
                    <?php } ?>
                </ul>
            </li>
        <?php } ?>
        <!-- Report menu end -->
        <!-- human resource management menu start -->
        <?php if ($this->permission1->method('add_designation', 'create')->access() || $this->permission1->method('manage_designation', 'read')->access() || $this->permission1->method('employee_salary_generate', 'read')->access() || $this->permission1->method('salary_advance_view', 'read')->access() || $this->permission1->method('add_employee', 'create')->access() || $this->permission1->method('manage_employee', 'read')->access() || $this->permission1->method('add_person', 'create')->access() || $this->permission1->method('add_loan', 'create')->access() || $this->permission1->method('add_payment', 'create')->access() || $this->permission1->method('manage_person', 'read')->access() || $this->permission1->method('add_attendance', 'create')->access() || $this->permission1->method('manage_attendance', 'read')->access() || $this->permission1->method('attendance_report', 'read')->access() || $this->permission1->method('add_benefits', 'create')->access() || $this->permission1->method('manage_benefits', 'read')->access() || $this->permission1->method('add_salary_setup', 'create')->access() || $this->permission1->method('manage_salary_setup', 'read')->access() || $this->permission1->method('salary_generate', 'create')->access() || $this->permission1->method('manage_salary_generate', 'read')->access() || $this->permission1->method('salary_payment', 'create')->access() || $this->permission1->method('add_expense_item', 'create')->access() || $this->permission1->method('manage_expense_item', 'read')->access() || $this->permission1->method('add_expense', 'create')->access() || $this->permission1->method('manage_expense', 'read')->access() || $this->permission1->method('add_ofloan_person', 'create')->access() || $this->permission1->method('add_office_loan', 'create')->access() || $this->permission1->method('add_loan_payment', 'create')->access() || $this->permission1->method('manage_ofln_person', 'read')->access()) { ?>
            <!-- Supplier menu start -->
            <li class="treeview <?php
                                if ($this->uri->segment('1') == ("designation_form") || $this->uri->segment('1') == ("designation_list") || $this->uri->segment('1') == ("salary_advance_view") || $this->uri->segment('1') == ("salary_pay_slip") || $this->uri->segment('1') == ("employee_salary_chart") || $this->uri->segment('1') == ("manage_salary_advance") || $this->uri->segment('1') == ("employee_salary_approval") || $this->uri->segment('1') == ("employee_form") || $this->uri->segment('1') == ("employee_salary_generate") || $this->uri->segment('1') == ("employee_list") || $this->uri->segment('1') == ("add_attendance") || $this->uri->segment('1') == ("attendance_list") || $this->uri->segment('1') == ("attendance_report") || $this->uri->segment('1') == ("add_beneficials") || $this->uri->segment('1') == ("manage_benefits") || $this->uri->segment('1') == ("salary_setup") || $this->uri->segment('1') == ("manage_salary_setup") || $this->uri->segment('1') == ("salary_generate") || $this->uri->segment('1') == ("manage_salary_generate") || $this->uri->segment('1') == ("salary_payment") || $this->uri->segment('1') == ("employee_salary_payment_view") || $this->uri->segment('1') == ("add_expense_item") || $this->uri->segment('1') == ("manage_expense_item") || $this->uri->segment('1') == ("add_expense") || $this->uri->segment('1') == ("manage_expense") || $this->uri->segment('1') == ("expense_statement") || $this->uri->segment('1') == ("add_officeloan_person") || $this->uri->segment('1') == ("add_office_loan") || $this->uri->segment('1') == ("add_office_loan_payment") || $this->uri->segment('1') == ("manage_office_loan_person") || $this->uri->segment('1') == ("office_loan_person_ledger") || $this->uri->segment('1') == ("office_loan_person_ledgerdata") || $this->uri->segment('1') == ("edit_office_loan_person") || $this->uri->segment('1') == ("add_person") || $this->uri->segment('1') == ("add_loan") || $this->uri->segment('1') == ("add_payment") || $this->uri->segment('1') == ("man... [truncated]
                                    echo "active";
                                } else {
                                    echo " ";
                                }
                                ?>">
                <a href="#">
                    <i class="fa fa-users"></i><span><?php echo display('hrm_management') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>
                <ul class="treeview-menu">
                    <?php if ($this->permission1->method('add_designation', 'create')->access() || $this->permission1->method('manage_designation', 'read')->access()  || $this->permission1->method('add_employee', 'create')->access() || $this->permission1->method('manage_employee', 'read')->access()) { ?>
                        <!-- Supplier menu start -->
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("designation_form") || $this->uri->segment('1') == ("designation_list") ||  $this->uri->segment('1') == ("employee_form") || $this->uri->segment('1') == ("employee_list")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>">
                            <a href="#">
                                <i class="fa fa-users"></i><span><?php echo display('hrm') ?></span>
                                <span class="pull-right-container">
                                    <i class="fa fa-angle-left pull-right"></i>
                                </span>
                            </a>
                            <ul class="treeview-menu">
                                <?php if ($this->permission1->method('add_designation', 'create')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("designation_form")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('designation_form') ?>"><?php echo display('add_designation') ?></a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('manage_designation', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("designation_list")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('designation_list') ?>"><?php echo display('manage_designation') ?></a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('add_employee', 'create')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("employee_form")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('employee_form') ?>"><?php echo display('add_employee') ?></a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('manage_employee', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("employee_list")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('employee_list') ?>"><?php echo display('manage_employee') ?></a>
                                    </li>
                                <?php } ?>

                            </ul>
                        </li>
                    <?php } ?>


                    <!-- ================== Attendance menu start ================= -->
                    <?php if ($this->permission1->method('add_attendance', 'create')->access() || $this->permission1->method('manage_attendance', 'read')->access() || $this->permission1->method('attendance_report', 'read')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("add_attendance") || $this->uri->segment('1') == ("attendance_list") || $this->uri->segment('1') == ("attendance_report") || $this->uri->segment('1') == ("edit_attendance")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>">
                            <a href="#">
                                <i class="fa fa-clock-o"></i><span><?php echo display('attendance') ?></span>
                                <span class="pull-right-container">
                                    <i class="fa fa-angle-left pull-right"></i>
                                </span>
                            </a>
                            <ul class="treeview-menu">
                                <?php if ($this->permission1->method('add_attendance', 'create')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("add_attendance")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('add_attendance') ?>"><?php echo display('add_attendance') ?></a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('manage_attendance', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("attendance_list")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('attendance_list') ?>"><?php echo display('manage_attendance') ?></a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('attendance_report', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("attendance_report")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('attendance_report') ?>"><?php echo display('attendance_report') ?></a>
                                    </li>
                                <?php } ?>
                            </ul>
                        </li>
                    <?php } ?>
                    <!-- ====================== Attendance menu end ================== -->

                    <!-- ========================== Payroll menu start =================== -->
                    <?php if ($this->permission1->method('add_benefits', 'create')->access() || $this->permission1->method('manage_benefits', 'read')->access() || $this->permission1->method('employee_salary_generate', 'read')->access() || $this->permission1->method('employee_salary_payment_view', 'read')->access() || $this->permission1->method('salary_advance_view', 'read')->access() || $this->permission1->method('add_salary_setup', 'create')->access() || $this->permission1->method('manage_salary_setup', 'read')->access() || $this->permission1->method('salary_generate', 'create')->access() || $this->permission1->method('manage_salary_generate', 'read')->access() || $this->permission1->method('salary_payment', 'create')->access()) { ?>
                        <!-- payroll menu start -->
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("add_beneficials") || $this->uri->segment('1') == ("salary_pay_slip") || $this->uri->segment('1') == ("manage_benefits") || $this->uri->segment('1') == ("employee_salary_approval") || $this->uri->segment('1') == ("employee_salary_chart") || $this->uri->segment('1') == ("employee_salary_generate") || $this->uri->segment('1') == ("employee_salary_payment_view") || $this->uri->segment('1') == ("salary_setup") || $this->uri->segment('1') == ("salary_advance_view") || $this->uri->segment('1') == ("manage_salary_advance") || $this->uri->segment('1') == ("manage_salary_setup") || $this->uri->segment('1') == ("salary_generate") || $this->uri->segment('1') == ("manage_salary_generate") || $this->uri->segment('1') == ("salary_payment") || $this->uri->segment('1') == ("payslip")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>">
                            <a href="#">
                                <i class="fa fa-paypal"></i><span><?php echo display('payroll') ?></span>
                                <span class="pull-right-container">
                                    <i class="fa fa-angle-left pull-right"></i>
                                </span>
                            </a>
                            <ul class="treeview-menu">
                                <?php if ($this->permission1->method('salary_advance_view', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("salary_advance_view")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('salary_advance_view') ?>"> <?php echo display('salary_advance_view') ?></a></li>
                                <?php } ?>
                                <?php if ($this->permission1->method('employee_salary_generate', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("employee_salary_generate")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('employee_salary_generate') ?>"> <?php echo display('employee_salary_generate') ?></a></li>
                                <?php } ?>

                                <?php if ($this->permission1->method('employee_salary_payment_view', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("employee_salary_payment_view")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('employee_salary_payment_view') ?>"> <?php echo display('employee_salary_payment_view') ?></a></li>
                                <?php } ?>


                            </ul>
                        </li>
                    <?php } ?>
                    <!-- =============================== Payroll menu end =================== -->

                    <!--  Personal loan start -->

                    <!-- loan end -->
                </ul>
            </li>
        <?php } ?>
        <!-- Human resource management menu end -->
