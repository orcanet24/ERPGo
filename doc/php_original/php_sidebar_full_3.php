        <!-- Bank menu start -->
        <?php if ($this->permission1->method('add_bank', 'create')->access() || $this->permission1->method('bank_list', 'read')->access()) { ?>
            <li class="treeview <?php
                                if ($this->uri->segment('1') == ("bank_form") || $this->uri->segment('1') == ("bank_list") || $this->uri->segment('1') == ("bank_ledger") || $this->uri->segment('1') == ("bank_transaction")) {
                                    echo "active";
                                } else {
                                    echo " ";
                                }
                                ?>">
                <a href="#">
                    <i class="ti-briefcase"></i><span><?php echo display('bank') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>
                <ul class="treeview-menu">
                    <?php if ($this->permission1->method('add_bank', 'create')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("bank_form")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('bank_form') ?>"><?php echo display('add_new_bank') ?></a></li>
                    <?php } ?>

                    <?php if ($this->permission1->method('bank_list', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("bank_list")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('bank_list') ?>"><?php echo display('manage_bank') ?></a></li>
                    <?php } ?>
                </ul>
            </li>
        <?php } ?>
        <!-- Bank menu end -->

        <!-- service menu start -->
        <?php if ($this->permission1->method('create_service', 'create')->access() || $this->permission1->method('manage_service', 'read')->access() || $this->permission1->method('service_invoice', 'create')->access() || $this->permission1->method('manage_service_invoice', 'read')->access()) { ?>

            <li class="treeview <?php
                                if ($this->uri->segment('1') == ("add_service") || $this->uri->segment('1') == ("manage_service") || $this->uri->segment('1') == ("edit_service") || $this->uri->segment('1') == ("add_service_invoice") || $this->uri->segment('1') == ("service_details") || $this->uri->segment('1') == ("manage_service_invoice") || $this->uri->segment('1') == ("edit_service_invoice")) {
                                    echo "active";
                                } else {
                                    echo " ";
                                }
                                ?>">
                <a href="#">
                    <i class="fa fa-asl-interpreting"></i><span><?php echo display('service') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>
                <ul class="treeview-menu">
                    <?php if ($this->permission1->method('create_service', 'create')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("add_service")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href="<?php echo base_url('add_service') ?>"><?php echo display('add_service') ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('manage_service', 'read')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("manage_service")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href="<?php echo base_url('manage_service') ?>"><?php echo display('manage_service') ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('service_invoice', 'create')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("add_service_invoice")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href="<?php echo base_url('add_service_invoice') ?>"><?php echo display('service_invoice') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('manage_service_invoice', 'read')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("manage_service_invoice")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href="<?php echo base_url('manage_service_invoice') ?>"><?php echo display('manage_service_invoice') ?></a>
                        </li>
                    <?php } ?>
                </ul>
            </li>
        <?php } ?>
        <!-- Comission end -->
        <!-- Quotation Menu Start -->
        <?php if ($this->permission1->method('add_quotation', 'create')->access() || $this->permission1->method('manage_quotation', 'read')->access()) { ?>
            <li class="treeview <?php
                                if ($this->uri->segment('1') == ("add_quotation") || $this->uri->segment('1') == ("manage_quotation") || $this->uri->segment('1') == ("quotation_to_sales")) {
                                    echo "active";
                                } else {
                                    echo " ";
                                }
                                ?>">
                <a href="#">
                    <i class="fa fa-book"></i><span><?php echo display('quotation') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>
                <ul class="treeview-menu">
                    <?php if ($this->permission1->method('add_quotation', 'create')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("add_quotation")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('add_quotation') ?>"><?php echo display('add_quotation') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('manage_quotation', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("manage_quotation")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('manage_quotation') ?>"><?php echo display('manage_quotation') ?></a>
                        </li>
                    <?php } ?>
                </ul>
            </li>
        <?php } ?>
        <!-- quotation Menu end -->
        <!-- Tax menu start -->
        <?php if ($this->permission1->method('add_incometax', 'create')->access() || $this->permission1->method('manage_income_tax', 'read')->access() || $this->permission1->method('tax_settings', 'create')->access() || $this->permission1->method('tax_report', 'read')->access() || $this->permission1->method('invoice_wise_tax_report', 'read')->access() || $this->permission1->method('tax_settings', 'read')->access() || $this->permission1->method('vat_tax_setting', 'read')->access()) { ?>
            <li class="treeview <?php
                                if ($this->uri->segment('1') == ("invoice_wise_tax_report") || $this->uri->segment('1') == ("tax_setting") || $this->uri->segment('1') == ("income_tax") || $this->uri->segment('1') == ("manage_income_tax") || $this->uri->segment('1') == ("tax_reports") || $this->uri->segment('1') == ("update_tax_setting") || $this->uri->segment('1') == ("vat_tax_setting")) {
                                    echo "active";
                                } else {
                                    echo " ";
                                }
                                ?>">
                <a href="#">
                    <i class="fa fa-money"></i><span><?php echo display('tax') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>
                <ul class="treeview-menu">
                    <?php if ($this->permission1->method('tax_settings', 'create')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("vat_tax_setting")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('vat_tax_setting') ?>"><?php echo display('vat_tax_setting') ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('tax_settings', 'create')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("update_tax_setting")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('tax_setting') ?>"><?php echo display('tax_settings') ?></a></li>
                    <?php } ?>
                </ul>
            </li>
        <?php } ?>

        <!-- return menu start -->
        <?php if ($this->permission1->method('add_return', 'create')->access() || $this->permission1->method('return_list', 'read')->access() || $this->permission1->method('supplier_return_list', 'read')->access() || $this->permission1->method('wastage_return_list', 'read')->access()) { ?>
            <li class="treeview <?php
                                if ($this->uri->segment('1') == ("return_form") || $this->uri->segment('1') == ("invoice_return") || $this->uri->segment('1') == ("invoice_return_details") || $this->uri->segment('1') == ("supplier_return") || $this->uri->segment('1') == ("supplier_return_details") || $this->uri->segment('1') == ("invoice_return_list") || $this->uri->segment('1') == ("invoice_return_search") || $this->uri->segment('1') == ("supplier_return_list") || $this->uri->segment('1') == ("supplier_return_search") || $this->uri->segment('1') == ("wastage_return_list")) {
                                    echo "active";
                                } else {
                                    echo " ";
                                }
                                ?>">
                <a href="#">
                    <i class="fa fa-retweet" aria-hidden="true"></i><span><?php echo display('return'); ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>
                <ul class="treeview-menu">
                    <?php if ($this->permission1->method('add_return', 'create')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("return_form")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a href="<?php echo base_url('return_form') ?>"><?php echo display('return'); ?></a></li>
                    <?php } ?>
                    <?php if ($this->permission1->method('return_list', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("invoice_return_list")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('invoice_return_list') ?>"><?php echo display('stock_return_list') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('supplier_return_list', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("supplier_return_list")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('supplier_return_list') ?>"><?php echo display('supplier_return_list') ?></a>
                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('wastage_return_list', 'read')->access()) { ?>
                        <li class="treeview <?php if ($this->uri->segment('1') == ("wastage_return_list")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            } ?>"><a
                                href="<?php echo base_url('wastage_return_list') ?>"><?php echo display('wastage_return_list') ?></a>
                        </li>
                    <?php } ?>
                </ul>
            </li>

        <?php } ?>

        <!-- return menu end -->
        <!-- Software Settings menu start -->
        <?php if ($this->permission1->method('manage_company', 'read')->access() || $this->permission1->method('manage_company', 'create')->access() || $this->permission1->method('add_user', 'create')->access() || $this->permission1->method('add_user', 'read')->access() || $this->permission1->method('add_language', 'create')->access() || $this->permission1->method('add_currency', 'create')->access() || $this->permission1->method('soft_setting', 'create')->access() || $this->permission1->method('add_role', 'create')->access() || $this->permission1->method('role_list', 'read')->access() || $this->permission1->method('user_assign', 'create')->access() || $this->permission1->method('sms_configure', 'create')->access()) { ?>
            <li class="treeview <?php
                                if ($this->uri->segment('1') == ("company_list") || $this->uri->segment('1') == ("edit_company") || $this->uri->segment('1') == ("add_user") || $this->uri->segment('1') == ("user_list") || $this->uri->segment('1') == ("language") || $this->uri->segment('1') == ("currency_form") || $this->uri->segment('1') == ("currency_list") || $this->uri->segment('1') == ("settings") || $this->uri->segment('1') == ("mail_setting") || $this->uri->segment('1') == ("app_setting") || $this->uri->segment('1') == ("add_role") || $this->uri->segment('1') == ("role_list") || $this->uri->segment('1') == ("edit_role") || $this->uri->segment('1') == ("assign_role") || $this->uri->segment('1') == ("sms_setting") || $this->uri->segment('1') == ("restore") || $this->uri->segment('1') == ("db_import") || $this->uri->segment('1') == ("editPhrase") || $this->uri->segment('1') == ("phrases")) {
                                    echo "active";
                                } else {
                                    echo " ";
                                }
                                ?>">
                <a href="#">
                    <i class="ti-settings"></i><span><?php echo display('settings') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>
                <ul class="treeview-menu">
                    <!-- Software Settings menu start -->
                    <?php if ($this->permission1->method('manage_company', 'read')->access() || $this->permission1->method('manage_company', 'create')->access() || $this->permission1->method('add_user', 'create')->access() || $this->permission1->method('manage_user', 'read')->access() || $this->permission1->method('add_language', 'create')->access() || $this->permission1->method('add_currency', 'create')->access() || $this->permission1->method('soft_setting', 'create')->access() || $this->permission1->method('back_up', 'create')->access() || $this->permission1->method('back_up', 'read')->access() || $this->permission1->method('restore', 'create')->access() || $this->permission1->method('sql_import', 'create')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("company_list") || $this->uri->segment('1') == ("edit_company") || $this->uri->segment('1') == ("add_user") || $this->uri->segment('1') == ("user_list") || $this->uri->segment('1') == ("language") || $this->uri->segment('1') == ("currency_form") || $this->uri->segment('1') == ("currency_list") || $this->uri->segment('1') == ("settings") || $this->uri->segment('1') == ("mail_setting") || $this->uri->segment('1') == ("app_setting") || $this->uri->segment('1') == ("editPhrase") || $this->uri->segment('1') == ("phrases")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>">
                            <a href="#">
                                <i class="ti-settings"></i> <span><?php echo display('web_settings') ?></span>
                                <span class="pull-right-container">
                                    <i class="fa fa-angle-left pull-right"></i>
                                </span>
                            </a>
                            <ul class="treeview-menu">
                                <?php if ($this->permission1->method('manage_company', 'read')->access() || $this->permission1->method('manage_company', 'update')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("company_list")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('company_list') ?>"><?php echo display('manage_company') ?></a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('add_user', 'create')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("add_user")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('add_user') ?>"><?php echo display('add_user') ?></a></li>
                                <?php } ?>
                                <?php if ($this->permission1->method('manage_user', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("user_list")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('ai_settings') ?>"><?php echo display('ai_setting') ?></a></li>
                                <?php } ?>
                                <?php if ($this->permission1->method('manage_user', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("user_list")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('user_list') ?>"><?php echo display('manage_users') ?> </a></li>
                                <?php } ?>
                                <?php if ($this->permission1->method('add_language', 'create')->access() || $this->permission1->method('add_language', 'update')->access()) { ?>
                                    <li
                                        class="<?php echo (($this->uri->segment(1) == "language" || $this->uri->segment('1') == ("editPhrase") || $this->uri->segment('1') == ("phrases")) ? "active" : '') ?>">
                                        <a href="<?php echo base_url('language') ?>">

                                            <?php echo display('language') ?>
                                        </a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('add_currency', 'create')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("currency_form")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('currency_form') ?>"><?php echo display('currency') ?> </a></li>
                                <?php } ?>
                                <?php if ($this->permission1->method('soft_setting', 'create')->access() || $this->permission1->method('soft_setting', 'update')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("settings")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>">
                                        <a href="<?php echo base_url('settings') ?>"
                                            class="<?php echo (($this->uri->segment(1) == "settings") ? "active" : null) ?>">
                                            <?php echo display('settings') ?>
                                        </a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('print_setting', 'create')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("print_setting")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('print_setting') ?>"><?php echo display('print_setting') ?> </a>
                                    </li>
                                <?php } ?>
                                <?php if ($this->permission1->method('mail_setting', 'create')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("mail_setting")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('mail_setting') ?>"><?php echo display('mail_setting') ?> </a>
                                    </li>
                                <?php } ?>
                                <li class="treeview <?php if ($this->uri->segment('1') == "app_setting") {
                                                        echo "active";
                                                    } else {
                                                        echo " ";
                                                    } ?>"><a href="<?php echo base_url('app_setting') ?>"><?php echo display('app_setting') ?> </a></li>
                            </ul>
                        </li>
                    <?php } ?>
                    <!-- Role permission start -->
                    <?php if ($this->permission1->method('add_role', 'create')->access() || $this->permission1->method('role_list', 'read')->access() || $this->permission1->method('edit_role', 'create')->access() || $this->permission1->method('assign_role', 'create')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("add_role") || $this->uri->segment('1') == ("role_list") || $this->uri->segment('1') == ("edit_role") || $this->uri->segment('1') == ("assign_role")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>">
                            <a href="#">
                                <i class="ti-key"></i> <span><?php echo display('role_permission') ?></span>
                                <span class="pull-right-container">
                                    <i class="fa fa-angle-left pull-right"></i>
                                </span>
                            </a>
                            <ul class="treeview-menu">

                                <?php if ($this->permission1->method('add_role', 'create')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("add_role")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('add_role') ?>"><?php echo display('add_role') ?></a></li>
                                <?php } ?>
                                <?php if ($this->permission1->method('role_list', 'read')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("role_list")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('role_list') ?>"><?php echo display('role_list') ?></a></li>
                                <?php } ?>
                                <?php if ($this->permission1->method('user_assign', 'create')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("assign_role")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('assign_role') ?>"><?php echo display('user_assign_role') ?></a>
                                    </li>
                                <?php } ?>


                            </ul>
                        </li>
                    <?php } ?>
                    <!-- Role permission End -->

                    <!-- sms menu end -->
                    <!-- Synchronizer setting start -->
                    <?php if ($this->permission1->method('restore', 'create')->access() || $this->permission1->method('sql_import', 'create')->access() || $this->permission1->method('sql_import', 'create')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("restore") || $this->uri->segment('1') == ("db_import")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>">
                            <a href="#">
                                <i class="ti-reload"></i> <span><?php echo display('data_synchronizer') ?></span>
                                <span class="pull-right-container">
                                    <i class="fa fa-angle-left pull-right"></i>
                                </span>
                            </a>
                            <ul class="treeview-menu">

                                <?php if ($this->permission1->method('restore', 'create')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("restore")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('restore') ?>"><?php echo display('restore') ?></a></li>
                                <?php } ?>
                                <?php if ($this->permission1->method('sql_import', 'create')->access()) { ?>
                                    <li class="treeview <?php if ($this->uri->segment('1') == ("db_import")) {
                                                            echo "active";
                                                        } else {
                                                            echo " ";
                                                        } ?>"><a href="<?php echo base_url('db_import') ?>"><?php echo display('import') ?></a></li>
                                <?php } ?>

                                <li class="treeview <?php if ($this->uri->segment('2') == ("backup_create")) {
                                                        echo "active";
                                                    } else {
                                                        echo " ";
                                                    } ?>"><a href="<?php echo base_url('dashboard/backup_restore/download_backup') ?>"><?php echo display('backup') ?></a>
                                </li>
                            </ul>
                        </li>
                    <?php } ?>
                    <!-- Synchronizer setting end -->

                </ul>
            </li>
        <?php } ?>
        <!-- Software Settings menu end -->
        <!-- custom menu start-->

        <?php
        $path = 'application/modules/';
        $map  = directory_map($path);
        $HmvcMenu   = array();
        if (is_array($map) && sizeof($map) > 0)
            foreach ($map as $key => $value) {
                $menu = str_replace("\\", '/', $path . $key . 'config/menu.php');
                if (file_exists($menu)) {

                    if (file_exists(APPPATH . 'modules/' . $key . '/assets/data/env')) {
                        @include($menu);
                    }
                }
            }
        ?>

        <!-- custom menu end -->


        <?php if ($this->session->userdata('isAdmin')) { ?>
            <li class="treeview"><a href="<?php echo base_url('addon/module/index') ?>"><i class="fa fa-adn"></i>
                    <span><?php echo display('addon'); ?></span></a></li>


        <?php } ?>



    </ul>
</div> <!-- /.sidebar -->